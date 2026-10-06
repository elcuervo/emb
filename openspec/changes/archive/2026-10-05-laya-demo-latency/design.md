# Design

See proposal.md — Why.

## Context

Measured against the deployed sandbox, so the numbers below are observations,
not estimates:

| call | questions | input tokens | `inference_ms` | `elapsed_us` | ms/token |
|---|---:|---:|---:|---:|---:|
| Inbox | 5 | 407 | 5300.5 | 5,340,671 | 13.0 |
| Quickstart | 3 | 257 | 2669.1 | 2,705,746 | 10.4 |
| Inbox, repeated | 5 | 407 | (cached) | **217** | — |

`EMB.INFO laya-real` reports `quantization: fp32`, `model_bytes: 846,075,712`,
`workers: 1`, `batch_determinism: failed`. `INFO cpu` reports `gomaxprocs: 4`.
`website/repl/sandbox.yaml` sets `intra_op_threads: 1` and
`inter_op_threads: 1`; `website/repl/fly.toml` sets `OMP_NUM_THREADS = "1"`.

The graph is a chain with two outputs, requested as `logits` and `act_logits`,
so inter-op threads buy nothing and intra-op threads are the whole lever. The
mapping from work to time is linear in tokens: `2 × 403e6 params × 407 tokens ≈
328 GFLOP` over 5300 ms is ~62 GFLOPS, which is a single modern core's fp32
throughput — a four-core box doing single-thread work.

### Measured on the deployed sandbox (2026-10-06, `shared-cpu-4x` / 8 GB)

| call | 1 thread (before) | 4 threads (after) |
|---|---:|---:|
| Inbox ticket 0, 407 tokens | 5300.5 ms | 2616.5 ms (2.0×) |
| Quickstart, 257 tokens | 2669.1 ms | 773.5 ms (3.5×) |
| Cold undeclared state, 417 tokens | — | 1626.4 ms (3.3× per token) |

A declared payload answers from the cache in ~250 µs, and the plate's `inference_ms`
on a hit is the warm run's real timing. On the first boot the warm stopped after
8 of 9 payloads because a client request was served mid-warm — the yield rule
working, triggered by a measurement request. Steady RSS after load is ~4.8 GB of
the machine's 8 GB (`mem: 4812`), with the restore path accounting ~3.9 GB; no
OOM, and no process-wide thread cap remains. The 1-thread vs 4-thread gap is
sub-linear and noisy on shared vCPUs, as expected.

Constraints that shape the warm:

- The reply cache key is `CacheKeyConfig(model, sha, args, len(texts), text,
  digest)`, where `text` is `KEYS[1]` (the state string) and `digest` folds in
  the script's declared config. A warm payload therefore reproduces a client's
  key exactly when it carries the same state text and the same `ARGV`.
- `cache_file: /data/cache.embcache` with `cache_save: 30m` and the default
  restore/save-on-shutdown means a warm survives restarts; only a fresh volume
  is cold.
- Fly's health check has `grace_period = "30s"`. The full nine-payload warm is
  on the order of 15 s at four threads and ~45 s at one, so a synchronous
  before-ready warm would fail the health check.
- The bridge serializes every upstream command on one connection and the model
  has one worker session, so a warm payload and a visitor's call cannot run
  concurrently: warming *is* occupying the capacity a visitor wants.

## Goals / Non-Goals

**Goals:**
- A cold typed-question call uses the machine, not one core.
- The plate's fixed payloads answer from the cache, with readiness unchanged.
- One source of truth for the payload list, testable without the sandbox.

**Non-Goals:**
- Quantized or otherwise smaller weights (no int8 Laya export exists, and its
  calibration drift needs its own fidelity check).
- Changing the preset, its answers, or its digest.
- A warm scheduler, priority queue, or per-visitor behavior.
- GPU machines.

## Decisions

### D1 — Size intra-op threads to the machine, keep one worker

Set `intra_op_threads` explicitly to the machine's vCPU count (4 on
`shared-cpu-4x`) and remove `OMP_NUM_THREADS=1`; leave `inter_op_threads: 1`
and `workers: 1`. The worker count is genuinely serialized by the bridge, but
the thread count parallelizes **within** one request, which is why the existing
pin's stated reason does not hold. The `inference-cpu-isolation` default of
`cores−2` exists to keep request parsing from starving ONNX on a busy
multi-connection server; this sandbox serves one serialized loopback connection,
so it can spend the cores on the model.

**Alternatives:** rely on the default (unset ⇒ `cores−2` = 2, ~2× instead of
~4×); raise `workers` instead (memory, and the bridge would still serialize);
leave it alone (the status quo, 5.3 s).

### D2 — The warm is declared server-side, next to the script preload

Extend the existing `scripts:` entry with a `warm:` list: each item is the
texts and arguments a client would send. After the model and scripts load, the
server evaluates each item through the ordinary scripted path (the same code the
reply cache already wraps), so the key is the client's key by construction and
no second cache mechanism exists. This keeps one source of truth (the config
beside `script_preload`) and is testable without the sandbox.

**Alternatives:**

- *A `just sandbox-warm` step that curls the payloads after deploy.* No Go
  change, but it duplicates the payloads a second time (the page already holds
  them), depends on the operator running it, and does nothing on an automatic
  machine replacement.
- *Prefetch from the page on mount.* No server change and the payloads already
  live in the page, but the first visitor still pays the cold cost (just
  overlapped with reading), it warms nothing for the standalone terminal, and it
  spends inference on visitors who never click.
- *Gate readiness on the warm.* Simple to reason about, but ~15 s of warmed
  work against a 30 s grace period leaves no margin and makes every boot longer
  than it must be.

### D3 — The warm stops as soon as a client is being served

A single worker means a warm payload delays a visitor's call by its whole
duration. Before each payload, compare the model's served-request count with the
value captured after the previous payload; if it moved, stop the warm. Remaining
payloads stay cold and answer exactly as today. This bounds a visitor's
worst-case wait to at most one in-flight warm payload (which for the payload
they themselves asked for is work that would have happened anyway).

**Alternative:** warm the whole set unconditionally. Simpler, but on a machine
replaced mid-day a visitor can land behind ~15–18 s of queued warm work.

### D4 — The sandbox declares the default-on-screen payloads first

Declare the Quickstart round and the Inbox ticket the plate opens on, then the
remaining tickets. Ordering matters because D3 can cut the warm short: the
payloads a visitor is most likely to hit are warmed before a visitor can arrive,
and the rest are opportunistic. The full set is nine payloads; if the sandbox
chooses to declare only the first two (~4 s at four threads), the rest cold-start
once and are cached afterwards.

## Risks / Trade-offs

- **Load-time RSS.** The old pin cites per-thread memory growth during graph
  load. Intra-op threads share the session's weights, so the growth should be
  arenas/activations rather than a second copy — but it must be measured on the
  8 GB machine (RSS at load and steady) before the change lands. → If it does
  not fit, fall back to `cores−2` or a smaller explicit value; the warm alone
  still removes the cost for visitors.
- **Warm-vs-visitor race** → D3 stops the warm on traffic; the remaining
  payloads answer cold.
- **Payload drift.** The declared payloads mirror strings in
  `website/demos/laya.html`. If the page's states or questions change, the warm
  computes replies for the old text and the new one cold-starts. → A drifted
  payload is a cache miss, never a wrong answer; note it beside the declaration
  and keep the page as the source the config mirrors.
- **A warm failure is silent to visitors.** → It is logged, and the payload
  still answers on demand (spec requirement), so a failed warm degrades to
  today's behavior rather than to an error.
- **Thread count is a host fact, not a constant.** Shared vCPUs are throttled;
  the 4-thread gain may be sub-linear. → Measure the real ratio; the decision is
  worth it even at 2×.

## Migration Plan

1. Land the config change (threads) alone and measure a cold Inbox call, plus
   load/steady RSS. Revert by restoring the two values.
2. Land the warm declaration and the server mechanism. With no `warm:` list, the
   server's behavior is unchanged, so this is additive; revert by removing the
   declarations.
3. `just sandbox-deploy` (the sandbox is hand-deployed; a merged push does not
   reach `cli.emb.is`). The volume already holds the checkpoint, so no download.
4. Verify with a fresh cache (delete `/data/cache.embcache` or exercise a
   payload the plate has not run): the plate's call should return the warm
   entry's `inference_ms` with a cache-hit-sized `elapsed_us`.

No preset bytes change, so the stamped SHA1s do not move and
`just website-presets-check` stays green.

## Open Questions

Resolved by the deployed measurements above: 4 intra-op threads are in use and
hold steady (~4.8 GB RSS, no OOM), so `cores−2` was not needed; and the sandbox
declares all nine payloads, with the warm's yield covering the traffic case.
