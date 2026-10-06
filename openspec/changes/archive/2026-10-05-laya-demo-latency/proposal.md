# Proposal

## Why

The decision plate's typed-question examples take about **5.3 seconds** per call
and it is almost entirely the model: the Inbox call returns
`inference_ms = 5300` of a `5,340,671 µs` response — 99.2% — for 407 input
tokens. That is ~13 ms/token from a ~403M-parameter checkpoint.

Two things make the wait both avoidable and worse than it looks:

- The sandbox pins the real checkpoint to one thread (`intra_op_threads: 1`,
  `inter_op_threads: 1`) and the image sets `OMP_NUM_THREADS=1`, while the
  machine reports `gomaxprocs: 4`. Three of four vCPUs are idle during the one
  operation that is the bottleneck. The pin's recorded reason — "the bridge
  serializes upstream work, so intra-op parallelism buys latency it cannot use"
  — does not hold: intra-op threads parallelize **within** one request's
  matmuls, which is exactly what is slow. The pin dates from the 2 GB landing
  (the config comment still says "this machine is 1.92 GiB"); the machine is now
  `shared-cpu-4x` / 8 GB.
- The reply cache already makes a repeat instant (measured 217 µs) and already
  survives restarts on the volume, but nothing warms it. Every visitor's first
  call on each of the plate's nine fixed payloads pays the full cold cost.

The archived `laya-real-checkpoint` design estimated "hundreds of milliseconds"
for this call (D4). The measured number is ~10–20× that, which is why the plate
has no label for the wait and why the copy's "repeated requests hit the reply
cache" only describes the fast path.

## What Changes

- **Use the machine's cores.** The real checkpoint is sized to the machine
  (`intra_op_threads` = cores) and the global single-thread pin is dropped,
  keeping `workers: 1` because the bridge serializes upstream work anyway.
  This is a configuration change only — no model, preset, digest, or answer
  changes, so calibration parity is untouched.
- **Declare the demo payloads and warm the reply cache after load.** The plate's
  fixed payloads (eight Inbox tickets and the Quickstart round) are declared
  where the preset is preloaded, and the server runs them once after the model
  is ready, so a visitor's call is a cache hit. The warm MUST NOT gate
  readiness: the platform health check's grace period is 30 s and the full warm
  is on the order of 15 s at several threads (and ~45 s at one).
- **Say what the wait is.** The plate states the real checkpoint's first-call
  cost and that later calls are cache hits, so the number on screen is not
  mistaken for a warm latency.
- **Not in this change:** quantized weights. The sandbox already supports int8
  (`quantize: auto`, `EMB.INFO`'s `quantization`), but `codenamev/laya-onnx`
  ships only fp16 `model.onnx` per subfolder and `laya-real` pins an explicit
  `onnx:` path, so int8 is an export-and-publish job plus a fidelity check
  against the fp16 answers — its own change. Also not in scope: swapping the
  checkpoint, and GPU machines.

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities

- `sandbox-service`: the sandbox SHALL NOT leave its cores idle for the
  latency-critical decision checkpoint, and SHALL serve the plate's fixed
  demo payloads from a warm cache after boot rather than paying the cold cost
  again for every visitor.
- `script-preload`: extends boot-time preload with a declared set of warm
  payloads whose replies are computed once after the model loads and left in
  the reply cache, without delaying readiness — the mechanism, so the warm has
  one source of truth and is testable without the sandbox.

## Impact

- **Config:** `website/repl/sandbox.yaml` (thread counts, declared warm
  payloads), `website/repl/fly.toml` (drop `OMP_NUM_THREADS=1`). The stale
  `cache:` comment's machine size is corrected in passing.
- **Server:** `internal/config` accepts the warm declaration; `internal/registry`
  (or the equivalent load path) runs it after the model is loaded; tests cover
  declaration, refusal of a malformed payload, and non-blocking readiness.
- **Site:** `website/demos/laya.html` copy only; the payloads it already carries
  are the warm inputs and stay the source the declaration mirrors.
- **Presets/digests:** untouched — `laya.lua` bytes are unchanged, so the
  stamped SHA1s do not move (`just website-presets-check` stays green).
- **Memory:** intra-op threads share the session's weights, so the model
  footprint should not grow per thread; the load-time arena growth and steady
  RSS SHALL be measured on the 8 GB machine before the change lands.
- **No new dependency, no download, no second checkpoint.**
