# Proposal

## Why

`emb` now ships good static capacity defaults (idle-first dispatch, `(cores−2)/script_workers` threads, shared sessions with 4 callers and spinning off). Those defaults are tuned for *concurrent* traffic, but one model can see different shapes in the same deployment: serial interactive calls, where the extra ~1 ms of spinning-off and the reduced per-session thread count hurt; bursty concurrent calls, where shared callers win; and cache-hit-heavy periods, where inference capacity does not matter at all. Operators should not have to predict which shape dominates. The server should observe it per model and adapt.

## What Changes

- **Per-model traffic classification.** From counters already shipped (`dispatch_wait_us`, `run_us`, `runs`, `sessions_busy/total`) plus a new per-model in-flight gauge sampled at the inference boundary, classify each model's recent traffic as `idle`, `latency`, `throughput`, or `saturated`. Cache hits never reach the inference boundary and are excluded.
- **Runtime-adaptive per-session concurrency.** `script_callers_per_session` becomes a *cap*, not a fixed value: a growable token pool (replacing the fixed buffered idle channel) lets an AIMD controller raise concurrency toward `min(observed in-flight, cap)` when tokens are exhausted and CPU has headroom, and lower it after sustained low pressure, with hysteresis.
- **CPU-saturation gate.** The controller never grows callers when process CPU is saturated; it logs a provisioning recommendation (raise `script_workers`/`intra_op_threads`) instead of adding concurrency that cannot help.
- **Capacity profiles.** A `capacity` setting (`auto` default, `latency`, `throughput`) selects a model's creation-time layout: `latency` pins one caller per session and ORT spinning on, `throughput` pins shared callers and spinning off, `auto` keeps the derived layout. The runtime controller only adapts callers; a persistent class/layout mismatch is logged as a recommendation, never reshaped under load.
- **Cgroup-aware sizing.** Session auto-tune reads the container memory limit (cgroup v2/v1) instead of host-wide memory, so Fargate/container sizing does not overshoot.
- **Observability and a kill switch.** Per model, `EMB.INFO`/`EMB.STATS` expose the current traffic class, in-flight count, current/target callers, and whether autotune is active; `autotune: off` disables the controller.
- **Traffic-shape benchmark.** `cmd/evalbench` gains a serial→burst→serial shape mode so adaptation is provable; `just bench-script` records before/after.

Not **BREAKING**: explicit `script_callers_per_session`, `allow_spinning`, `intra_op_threads`, and `script_workers` keep their meaning; `capacity` and `autotune` default to `auto` and can be disabled.

## Capabilities

### New Capabilities

- `inference-capacity-autotune`: per-model traffic classification, the runtime concurrency controller and its safety bounds, capacity profiles, and the observable autotune state.

### Modified Capabilities

- `script-inference-performance`: `script_callers_per_session` becomes a cap with an adaptive default; spinning follows the selected or observed class instead of a fixed shared rule.
- `inference-performance`: session auto-tune becomes cgroup-aware; the unbatched embed pool participates in the same concurrency controller.
- `server-stats-observability`: report traffic class, in-flight, current/target callers, and autotune state per model.

## Impact

- **Code:** `internal/config` (capacity/autotune fields), `internal/registry` (classifier, controller, cgroup memory), `internal/pipeline` (growable token pool and per-model in-flight at the inference boundary), `internal/server` (script/embed boundary gauge, INFO/STATS fields), `cmd/evalbench` (shape mode).
- **Unchanged:** wire protocol, reply format, cache keys, and returned embeddings.
- **Configuration:** new optional `capacity` and `autotune` keys. Defaults preserve today's behavior except that callers may grow at runtime; explicit values are still honoured verbatim.
