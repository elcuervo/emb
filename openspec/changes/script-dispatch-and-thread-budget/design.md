## Context

Measurements come from the GLiNER2 migration in the Unsplash API, run against emb 0.4.3.pre1 with ORT 1.30 on an Apple M-series (10 cores). The corpus is 36 phrases with 5 labels, and the reply cache was off.

Breakdown of a serial request at 4×2 (instrumented build, 360 requests):

| Phase | µs |
|---|---|
| RESP parse, SHA1, script cache lookup | 12 |
| Fresh Lua state | 86 |
| `tokenize.words` + `tokenize.pretokenized` | 116 |
| Lua → Go tensors | 5 |
| **ORT `Run`** | **22,091 (97.4%)** |
| Output copy + pack | 34 |
| gather + sigmoid | 48 |
| JSON decode + encode | 26 |
| Lua VM self time | ~230 |
| Reply | 3 |

At c=4 to c=16, the mean session-lock wait was 37.7 ms and ORT run time grew to 28.8 ms from CPU contention.

Serial ORT time for one session by `intra_op_threads`: 2 → 21.8 ms, 4 → 14.8, 8 → 13.9, 10 → 23.0 (oversubscribed).

Layouts (workers × threads) at c=8:

| Layout | serial p50 | c=4 p50 | req/s | p99 |
|---|---|---|---|---|
| 8×1 | 38 ms | 39 ms | 184 | 72 ms |
| 4×2 | 23.6 ms | 26.7 ms | 143 | 103 ms |
| 2×4 | 16 ms | 38 ms | 98 | 154 ms |
| 1×8 | 13–17 ms | 58 ms | 63 | 268 ms |
| 4×default(8) | 29 ms | – | 24 | 865 ms |

Production load for GLiNER: about 56 req/s on average, 112 req/s for the busiest minute, across 8 emb tasks of 8 vCPU each.

## Decisions

**1. Idle-first dispatch with a buffered channel of sessions.**
`ScriptResources` holds `idle chan onnx.NamedSession`, filled with every session at open. `RunNamed` takes a session from the channel, runs, and returns it.

- Fairness is FIFO on the channel receive.
- There is no extra lock, and an idle session is never skipped.
- `Session()` stays for callers that need a specific instance (Close, tests).

*Alternative:* least-loaded counters plus a mutex per session. Rejected: more code, same effect.

*Alternative:* bigger pools. Rejected: each session costs a model footprint (~350 MB for GLiNER2 int8).

**2. Embedding and image pools use the same mechanism, gated by measurement.**
The unbatched `pipeline.Pool` sends to `workers[idx].reqChan`. The equivalent change is one shared request channel that all workers read. That is a work-conserving queue with the same semantics as Decision 1.

`image.go` mirrors `ScriptResources` and takes Decision 1 directly.

The change ships only if siglip2 c=8 / c=16 p99 improves with no serial regression. Production runs this path with batching off, so it is the most likely cause of the siglip2/E5 p99 being worse than in-process.

**3. Thread default divides the budget.**
`defaultIntraOpThreads()` stays `cores − 2` for a single session. Script sessions use `max(1, (cores − 2) / script_workers)` when `intra_op_threads` is unset.

An explicit `intra_op_threads` is never changed. The boot log warns when `Σ sessions × threads` across loaded models exceeds `GOMAXPROCS`, naming the models involved.

*Alternative:* a global ORT thread pool shared by all sessions. This is deferred pending the ORT options study; it needs `DisablePerSessionThreads` and changes isolation between models.

**4. Observability is counters, not histograms.**
Each model gets atomic cumulative `dispatch_wait_us`, `run_us` and `runs`, plus gauges `sessions_busy` and `sessions_total`, split into script and embed.

Averages come from deltas between two `EMB.STATS` reads. Datadog can scrape these via the existing stats exporter path, or via a sidecar that diffs counters.

- Read cost is a few atomic loads.
- There is no allocation on the hot path beyond two `time.Now()` calls.

**5. Benchmark harness.**
Promote the profiling load client to a committed command (`cmd/evalbench`). Its inputs are a corpus file, a script, labels, a concurrency list, and the request count. It reports p50/p90/p99 and req/s per concurrency level, and dumps replies.

A `just bench-script` target compares a candidate against a baseline binary (interleaved runs) and fails on any reply mismatch.

**6. Concurrent callers on shared sessions, spinning off.**
ORT sessions are thread-safe for `Run`. Today `NamedRuntimeSession` serializes runs with a mutex because it keeps a cache of output tensors keyed by shape. Change this:
- Drop the mutex and allocate outputs per call. GLiNER's only output is `[1, seq, 8, nlab]`, 1–7 KB per call.
- `script_callers_per_session` (default 1) puts each session into the idle channel that many times, so Decision 1 also governs callers.
- `allow_spinning: false` sets `session.intra_op.allow_spinning=0`.

With spinning on, concurrent callers fight for cores: 1 session × 8 threads with 8 callers falls to 97 req/s. With spinning off, they share the pool cleanly.

Measured, Mac, 8 threads total:

| Layout | c=1 p50 | c=4 p50 | req/s | sessions |
|---|---|---|---|---|
| 4×2 (current) | 23.0 | 29.2 | 133 | 4 |
| 4×2, spinning off | 24.4 | 27.9 | 145 | 4 |
| 2×4, 4 callers, spinning off | 19.4 | 26.7 | 164 | 2 |
| 1×8, 4 callers, spinning off | 18.7 | 29.5 | 146 | 1 |
| 2×4, 4 callers, spinning on | 15.8 | 30.0 | 118 | 2 |

ORT session options in the gem and in emb are otherwise the same: opt level ALL, sequential, arena and memory pattern on. Raw ORT time is about the same in both (gem 13.9 ms, probe 11.5–14 ms), so emb overhead at 1×8 is about 1.5–2 ms.

*Alternative:* a global ORT thread pool (`CreateEnvWithGlobalThreadPools`, `DisablePerSessionThreads`). Not exposed by `onnxruntime_go`; shared sessions give the same result.

## Risks / Trade-offs

- **Spinning off adds about 1 ms serial latency** when there is a single caller. It stays opt-in, recommended only with shared callers.
- **Lower default threads per script session make serial latency worse** for operators who relied on the old default with `script_workers > 1`. That setup was already oversubscribed, so throughput and tail latency improve. The change is documented in configuration docs and the changelog.
- **A shared queue in the embedding pool changes which worker serves a request.** Outputs are deterministic per session, and sessions are identical, so replies are unaffected. Parity is verified by the benchmark dump.
- **Benchmarks on a laptop don't predict Fargate x86.** Production numbers come from the new `EMB.STATS` counters and the API's `backend` span tag. Local budgets are relative to the baseline binary, not absolute.

## Open Questions

- Production layout: 2×4 shared with spinning off (best latency and memory on Mac), 8×1 (best tail), or 4×2? A production 8 vCPU Fargate task is probably 4 physical cores with hyperthreading. Re-run the comparison with `bench/fargate` before choosing, and check against the production `dispatch_wait_us`.
- Should spinning off become the default when `script_callers_per_session > 1`? It costs about 1 ms serial for a single caller.
