# Design

## Context

The just-archived `script-dispatch-and-thread-budget` change shipped the static capacity defaults: idle-first dispatch, `(cores−2)/script_workers` threads, shared `NamedRuntimeSession` runs, `script_callers_per_session` (default 4), and `allow_spinning` off while a session is shared. It also shipped the per-model counters this change consumes: `dispatch_wait_us`, `run_us`, `runs`, `sessions_busy`, `sessions_total`.

Three facts shape the approach:

- **Mutability.** Session count, per-session intra-op threads, and ORT `allow_spinning` are fixed when a session is created. Only *how many runs share a session* (concurrency) can change at runtime without rebuilding sessions.
- **Signals.** Per model we already have cumulative dispatch wait and run time plus live busy/total gauges; server-wide we have `cpu_user_usec`/`cpu_sys_usec` and `GOMAXPROCS`. Cache hits return before any inference runs.
- **Constraints.** Auto-tuned `script_workers` is bounded by the embedding pool's session count; batching is determinism-gated; containers (Fargate 8 vCPU, cgroup limits) may not match host-reported CPU/memory.

See `proposal.md — Why` for motivation.

## Goals / Non-Goals

**Goals:**

- Classify each model's recent traffic per path (script, unbatched embed) into `idle | latency | throughput | saturated`.
- Adapt per-session concurrency within the configured cap, safely, with a kill switch.
- Let operators pin a creation-time layout with a `capacity` profile.
- Size sessions from container limits, not the host.
- Expose enough state that an operator can see and trust the tuning.

**Non-Goals:**

- Rebuilding sessions or changing thread counts at runtime.
- A global ORT thread pool, graph-option changes, or batching changes.
- Tuning the batcher pool (single session; concurrency is meaningless there).
- Client-side or wire-protocol changes.

## Decisions

**1. Measure in-flight at the inference boundary.**
Add an atomic in-flight gauge incremented around `ScriptResources.RunNamed` and around the worker-pool handoff in `pipeline.Pool.Embed`. Cache hits return before these points, so classification never counts them, and the gauge is exactly "runs in flight".
*Alternative:* count at the RESP handler — rejected, it counts cache hits and would make a cache-heavy model look busy.

**2. Classify from queueing, not request rate.**
A background sampler per model (one ticker, default 1 s, stopped on close) reads the counters and computes, for the window: `runsΔ`, `waitPerRun = Δdispatch_wait_us/Δruns`, `runPerRun = Δrun_us/Δruns`, `peakInFlight`, and `cpuUtil = Δ(cpuUserUs+cpuSysUs) / (window × GOMAXPROCS)`. Classification:

```
runsΔ == 0                      → idle
cpuUtil ≥ 0.90                  → saturated
peakInFlight > sessions_total
   || waitPerRun/runPerRun ≥ 0.25 → throughput
otherwise                       → latency
```

*Alternative:* per-request latency histogram — rejected; allocation and state for no extra signal.

**3. AIMD on a resizable permit pool, not the channel.**
Replace the fixed buffered `idle chan` with a small permit structure over the fixed set of sessions: `Acquire` returns the least-recently-used session, `Release` returns it, and `SetCapacity(n)` adds or removes permits (`1 ≤ n ≤ cap`). Growing adds permits; shrinking removes *free* permits only — an in-flight permit is never revoked and the capacity takes effect as it returns.
Control law: expand the allowance straight to the cap after 1 `throughput` window; halve it after 10 consecutive `latency` windows (≈10 s). An `idle` window holds (no traffic is not evidence of a latency-sensitive workload). At most one change per window. Fast expansion keeps a burst from queueing behind a low allowance; slow contraction keeps a short serial lull from starving the next burst.
*Alternative:* rebuild the buffered channel — impossible; channels cannot be resized.
*Alternative:* `golang.org/x/sync/semaphore` — grows but cannot shrink to a new ceiling cleanly and adds a dependency.

**4. CPU-saturation gate and provisioning recommendation.**
Never grow while `cpuUtil ≥ 0.90`: more callers cannot create CPU. On the transition into `saturated`, log once per model a recommendation to raise `script_workers` (more sessions) or `intra_op_threads` (wider sessions). This distinguishes "capacity mis-sized" from "concurrency under-shared".
*Alternative:* grow regardless — rejected (oscillation, no throughput gain).

**5. Profiles pick the creation-time layout; there is no runtime reshape.**
`capacity: auto|latency|throughput` maps to `(spinning, initial callers, controller)`:

| capacity | spinning | initial allowance | controller |
|---|---|---|---|
| `auto` (default) | off when shared | derived (≤ cap 4) | on |
| `latency` | on | 1 | off (fixed) |
| `throughput` | off | cap | on |

A persistent `latency`-class model whose profile is `throughput` gets a recommendation, not a rebuild.
*Alternative:* double-buffer and swap sessions on class change — rejected (memory ×2, disruptive under load).
*Alternative:* rebuild sessions on a long-window class change — deferred (Non-Goal).

**6. Container-aware sizing.**
A new helper resolves the effective CPU/memory budget from cgroup v2 (`memory.max`, `cpu.max`) then v1 (`memory.limit_in_bytes`, `cpu.cfs_quota_us/period`), falling back to host values. `autoTuneWorkers` uses the effective memory limit; the derived thread count uses the effective core count. This fixes the container case where host memory makes `autoTuneWorkers` overshoot.
*Alternative:* rely on `GOMAXPROCS` alone — already cgroup-aware for CPU, but not for memory.

**7. Minimal config surface.**
Add `capacity` and `autotune` (`off|callers|auto`, default `auto`). Keep `script_callers_per_session` as the cap. Thresholds (`window=1s`, `cpuHigh=0.90`, `waitRatio=0.25`, grow/shrink dwell) are documented constants, not config, to avoid a tuning matrix.
*Alternative:* expose every threshold — rejected; adds config surface with no operator ask.

**8. Observability.**
Per model, add `traffic_class`, `inflight`, `concurrency_current`, `concurrency_target`, and `autotune_active` to `EMB.INFO` and the per-model `EMB.STATS` strings; update RESP array counts. The sampler owns the values; reads are atomic loads.

**9. Benchmark the shape, not just the layout.**
`cmd/evalbench` gains `-shape serial|burst|mixed`, running fixed phases (c=1 → c=C → c=1) and sampling `EMB.INFO` to show the class and allowance move. `just bench-script` records the shape run in `BENCHMARK.md`.

## Risks / Trade-offs

- **Oscillation under alternating load** → grow/shrink require consecutive agreeing windows, one change per window, and shrinking halves.
- **First burst is served at the initial allowance** → the initial allowance stays the derived/cap value, so this is a single window at worst; no cold-start starvation.
- **A cache-heavy workload hides its miss load** → deliberate: the sampler only sees inference, so it tunes the work that actually runs.
- **Sampler goroutine per model** → one ticker per model, started lazily with the resources and stopped on close; bounded by model count.
- **Physical vs logical cores is not portable from Go** → the effective core count and the −2 reserve only approximate it; documented as a known limit, and the oversubscription warning already names offenders.
- **Adaptation can mask a genuinely undersized model** → the `saturated` class explicitly recommends resizing instead of silently adding concurrency.

## Migration Plan

Additive and reversible: new optional config keys; defaults preserve current behavior apart from allowing concurrency to grow. Roll back with `autotune: off`. No data or wire changes. Verify on deploy by watching `EMB.INFO` class/allowance on GLiNER2 and siglip2 and comparing the `bench-script` shape run to the recorded baseline.

## Open Questions

- The exact `cpuHigh`, window, and dwell constants want one measurement pass on the reference host; they are tunable constants, so resolving them does not change the specs, approach, or task breakdown.
- Whether the provisioning recommendation should also become a first-class `EMB.STATS` field (currently a boot/transition log plus `EMB.INFO` state) can be decided when an operator asks.
