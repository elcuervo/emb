## Why

Scripted inference (GLiNER2 testbed) is CPU-bound and serialized in ways that benchmarking showed are removable. On Apple M4 with the int8 model: **16 req/s** single-connection (62 ms/req) with concurrent connections scaling only **1.5× for 4×** because all scripted evaluations share one mutex-serialized ORT session; multi-text `EMB.EVSHA` calls run N serialized inferences (~62 ms each); every request recompiles the Lua script and the first request pays ~300-500 ms of session creation. Entity extraction at interactive rates is fine, but batch and concurrent load need the server-side levers.

## What Changes

- **Per-model scripted session pool** (`script_workers` config, auto-tuned by RAM like the embed worker pool): N named-tensor sessions per model, distributed round-robin, so concurrent distinct-text requests run in parallel instead of queueing on one session.
- **`emb.run_batch` host block**: an array of input-spec tables → one padded inference run → per-item outputs. Scripts batch all texts of a request (the example `gliner2.lua` is updated to build one batch and decode per item). Purely additive; `emb.run` unchanged.
- **Compiled-script cache**: script source compiled once per (model, SHA) into bytecode (gopher-lua `CompileString`/`NewFunctionFromProto`), reused by every fresh per-eval state; `EMB.SCRIPT FLUSH` invalidates it. Sandbox/isolation semantics unchanged (fresh state per evaluation remains).
- **`script_preload` model config**: warm the scripted session + tokenizer at startup, eliminating first-request session-creation latency for declared scripted models.
- **`just bench-gliner`** extended with a multi-text/batched cell so the optimization is measured before/after (benchmarks exist from the baseline in `internal/script/gliner_bench_test.go`).
- Long-document chunking is **not** in scope (deferred rung, see design).

## Capabilities

### New Capabilities
- `script-inference-performance`: parallel scripted execution via the per-model session pool, batched scripted inference via `emb.run_batch`, and compiled-script reuse for repeat `EMB.EVSHA` executions.

### Modified Capabilities
- `model-loading`: `ModelConfig` gains `script_workers` (parallel scripted sessions; auto-tune by RAM when unset) and `script_preload` (warm scripted resources at startup) — mirroring the existing embed worker pool semantics.

## Impact

- **`internal/script`**: `run_batch` host block; per-model compiled-script cache wired into `EvalWithHosts`; tests.
- **`internal/registry`**: scripted session pool (N sessions, round-robin, auto-tune), `ScriptResources` becomes pool-aware; `script_preload` hook in `LoadModel`.
- **`internal/config`**: `ModelConfig.ScriptWorkers`/`ScriptPreload` keys.
- **`internal/server`**: `script_workers`/`script_preload` flow through; no reply-shape or command changes.
- **`examples/scripts/gliner2.lua`**: batch all KEYS texts into one `emb.run_batch` (reply shape unchanged; golden tests must stay green).
- **`justfile`**: benchmark coverage for the batched path.
- **Dependencies**: none new (gopher-lua already present).