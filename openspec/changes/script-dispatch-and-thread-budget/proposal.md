## Why

GLiNER2 (`gliner2.lua`, int8 graph with `DynamicQuantizeLinear`) is moving from an in-process Ruby gem to emb. A model that ran in-process at a flat 58 / 68 / 93 ms p50 / p90 / p99 in production must not get a worse tail on emb. Today it would:

- **Queueing, not compute, dominates under load.** Profiling the script path shows ORT `Run` is 97.4% of a serial request (22.1 of 22.7 ms); all emb overhead (Lua state, tokenize, marshalling, gather/sigmoid, JSON) is about 0.6 ms. At concurrency 8, requests waited a mean **37.7 ms** for a session lock, longer than inference itself. `ScriptResources.Session()` picks round-robin, so a request can queue behind a busy session while another sits idle.
- **The default thread count oversubscribes the CPU.** `intra_op_threads` defaults to `cores − 2` *per session*. With `script_workers: 4` on 10 cores that is 32 ORT threads, and throughput fell to ~24 req/s at any concurrency. Setting `4 × 2` explicitly gives ~120 req/s.
- **Production has no visibility into either.** Datadog has no emb-side queue depth, wait time or inference time. The siglip2/E5 tail (p99 ~119 ms vs ~87 ms in-process, and p90 still about +80% since 0.4.x) can't be attributed to queueing or compute.

The unbatched embedding `Pool` (production runs with batching disabled since 0.3.0) and the image pool use the same round-robin pick, so the same fix probably also addresses the existing siglip2/E5 tail. That part is a hypothesis this change measures.

## What Changes

- **Idle-first dispatch for script sessions.** `emb.run` / `emb.run_batch` take whichever named-tensor session is free first, instead of a round-robin index. Prototype (about 15 lines, `perf/script-probe`), 4×2, concurrency 8: p90 97 → 69 ms, p99 133–188 → 103 ms; at concurrency 16, p99 216–288 → 166 ms; +3–8% throughput; replies byte-identical; serial latency unchanged.
- **Idle-first dispatch for unbatched embedding and image pools.** Same policy for `pipeline.Pool` without a batcher and for `registry/image.go`. Adopt only if the measurement gate shows a tail improvement with no serial regression.
- **Thread-budget-aware default.** When `intra_op_threads` is unset, script sessions default to `max(1, (cores − 2) / script_workers)`. An explicit value is honoured verbatim, but emb logs a boot warning when the total thread budget (sessions × threads across all models) exceeds the core count.
- **Dispatch observability.** `EMB.STATS` / `EMB.INFO` report per-model session wait time, ORT run time, and busy/total sessions, for both the script and embedding paths, so the effect can be measured in production.
- **Committed benchmark** (`cmd/evalbench` or a `just` target): serial and concurrency 4 / 8 / 16 latency percentiles plus throughput for a script model, with a byte-for-byte reply parity check against the previous binary.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `script-inference-performance`: idle-first session dispatch; thread-budget default for script sessions; tail-latency budget under concurrency.
- `inference-performance`: idle-first dispatch for unbatched embedding and image pools; boot warning on thread oversubscription.
- `server-stats-observability`: per-model dispatch wait, run time and session occupancy.

## Impact

- **Code:** `internal/registry/registry.go` (`ScriptResources`, default threads, boot warning), `internal/server/script.go`, `internal/pipeline/pool.go`, `internal/registry/image.go`, stats/info reporting, plus a benchmark command.
- **Unchanged:** wire protocol, reply format, cache keys and model outputs.
- **Configuration:** an unset `intra_op_threads` with `script_workers > 1` now yields fewer threads per session, which raises throughput. Explicit configs behave as before.

## Measuring it

| Where | What | Pass |
|---|---|---|
| Local, GLiNER corpus (36 texts, `ent_labels`), 4×2, reply cache off | p50 / p90 / p99 at serial, c=4, c=8, c=16; req/s | c=8 p99 ≤ 110 ms; serial p50 within ±5% of baseline; req/s not lower |
| Local, same | Replies vs 0.4.3 release binary | Byte-identical, 36/36 |
| Local, unset `intra_op_threads`, `script_workers: 4` | req/s at c=8 | ≥ 4× the 0.4.3 default (~24 req/s) |
| Local, siglip2 unbatched pool | c=8 / c=16 p99 | Improves or stays within 5%; adopt only if it improves |
| `EMB.STATS` | `session_wait_us`, `run_us`, `sessions_busy` | Present per model; wait ≈ 0 when serial |
| Production (Datadog) | `phrase.entities` `backend:remote` vs `local` (58 / 68 / 93 ms); `phrase.embeddings` p90 / p99 before and after; emb `session_wait_us` | Remote p99 ≤ local p99; embeddings p90 back to the 0.3.x level (~42–56 ms at peak hours) |

## Non-goals

- Batching for dynamic-quantization graphs. This needs a static-QDQ int8 re-export of GLiNER2, which changes outputs (separate change).
- Rewriting Lua hot loops as Go builtins, IOBinding, partial logits reads or a faster JSON encoder. All of these together are bounded by about 0.6 ms of the 22.7 ms request.
- ORT session option changes (`allow_spinning`, `dynamic_block_base`, fp32). Measured with no gain, or worse.
- Client reconnect behaviour during deploys (the `ConnectionError`s seen on Oct 7). That is a client change.
