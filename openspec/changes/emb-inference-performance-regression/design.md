# Design

## Context

See `proposal.md` — Why, and "What the code shows" for the mechanism table. What shapes the approach:

- The comparison boundary is frozen: `a0ffc5b` (pre3, works) versus `0ab054e` (pre4, fails). `git diff a0ffc5b..0ab054e -- internal/pipeline internal/permit/pool.go internal/permit/autotune.go` is empty, so the runtime delta is one new object and the policy around it: `internal/permit/budget.go` (a FIFO `sync.Cond` gate), `internal/registry/cpu_budget.go` (wrappers that charge it), `internal/registry/calibration.go`, and the layout defaults in `registry.go`.
- The gate is process-wide and shared by embedding, scripted (`EMB.EVAL`/`EMB.EVSHA`), image and calibration runs (`registry.go:330,711,876`; `image.go:259`). Its wait has no deadline, no context and no queue bound (`budget.go:29-52`), and it admits only the head of the queue (`budget.go:36`).
- Nothing on the inference route carries a `context`: the redcon handler has none (`internal/server/server.go:363-408`), `Pool.Embed`/`Batcher.Embed` block on channels, and `Budget.Acquire` blocks on the condition variable. `max_concurrent_requests` defaults to `0` = unlimited (`internal/config/config.go:46`), so there is no backpressure either.
- The repo already states two of the required behaviors and does not implement them: "Idle model capacity SHALL be borrowable" and "The controller SHALL report admission wait separately from preprocessing, local pool wait, and run time" (`openspec/specs/inference-cpu-budget/spec.md`, Shared inference admission).
- Existing tooling to build on, not duplicate: `just bench-host-check` + the host contract, the recorded evidence bundle with per-cell metrics and assertion outcomes, the same-host fingerprint refusal rules, and `bench/smoke/verify-gate.sh` (mutant sweep) — all from the archived `performance-insight-and-smoke-gate` change. The scripted-path gate it added drives `EMB.EVSHA` on `testdata/laya`, which cannot see a plain-`EMB` admission stall.

## Goals / Non-Goals

**Goals:**

- Make the plain embedding path's latency a bounded function of load rather than of queue depth: overload is refused or delayed within a bound, never queued without limit.
- Restore the property the incident removed — concurrent work overlaps instead of parking behind one slow run — while keeping the process inside its CPU budget.
- Make the layout and the wait attributable: an operator can see the chosen sessions/threads/share, the admission wait, and whether the CPU was throttled.
- Leave a gate that is proven to fail on the regressions this change is about, and that separates the frozen pair on a qualified host.

**Non-Goals:**

- Fixing `EffectiveNumCPU()`'s sub-one-CPU flooring (Fly `shared-cpu-*`), which is a separate change; this one may report it.
- Absolute throughput targets or cross-host comparisons; the differential is same-host only.
- Re-recording the existing golden baselines or changing `EMB_BENCH_BUDGETS`.
- Changing the RESP surface, embeddings, or the Ruby client API.
- Selecting a runtime fix before the reproduction names the mechanism; the gate is built from the class of failure, which the frozen pair alone exposes.

## Decisions

### D1 — Admission carries a deadline and a context

`Budget.Acquire` gains a context/deadline and returns `(release, error)`; the caller distinguishes "admitted" from "timed out". The context originates at the command handler, so a client disconnect or a server-side bound ends the wait. The wrapper that holds the gate (`budgetedSession.Run`, `budgetedNamedSession.RunNamed`) is the only place that can time the wait, so the accounting lands there.

Alternatives: a longer timeout (does not bound anything, and the incident's number is the *client's* timeout); cancel at the pool boundary only (the batcher's `flush` runs the batch synchronously, so the gate wait would still be the unbounded segment).

**Transport limitation found while implementing (task 2.4).** The deadline half of D1 is implemented and verified; the *client-disconnect* half is not reachable with the current transport. `redcon` exposes the connection (`Conn.NetConn()`), but it executes a connection's commands serially on the goroutine that also reads it, so while a handler is inside inference nothing is reading the socket: a peer's FIN stays in the kernel buffer and a mid-request disconnect is not observable without stealing pipelined bytes from redcon's reader. What the server can and does do is bound the wait from its own deadline, drop work whose deadline expired before it ran, and treat the reply-write failure as the disconnect signal (redcon closes the connection then). True disconnect cancellation needs a transport change — a per-connection context delivered by redcon, or a Linux `POLLRDHUP` watcher — which is a separate decision, not a silent omission. Task 2.4 stays open for that reason.

The scripted (`emb.embed`) and image paths call the same controller but pass a background context today (their host API has no context to thread), so their admission wait is bounded by the queue bound only; threading their own deadline is a follow-up recorded in task 9.1.

### D2 — Overload is refused, not absorbed

The controller bounds its waiter queue and answers the existing busy shape (`ERR busy: …`, `server.go:167-185`) past it; the outer `max_concurrent_requests` keeps its meaning, but its default becomes a finite value that is documented and config-checked. Refusals are counted and visible, and health/introspection commands stay exempt as they are today.

Alternatives: unbounded queueing with a shorter deadline (still turns a spike into a latency cliff); refusing on the client (the server is what must not collapse; the web tier cannot distinguish this from a hang).

### D3 — Admission is admit-if-fits, with a reserved slot for narrow waiters

A waiter is admitted when its width fits current usage, even if an earlier, wider waiter is still blocked; a reserved slot (one admission unit reserved below the process budget when the budget admits more than one run) keeps narrow requests moving. This is what the spec's "idle capacity borrowing" requires and what the FIFO head rule violates.

Alternatives: strict FIFO (status quo — head-of-line blocking by construction); weighted fair queueing per model share (correct but larger than the demonstrated need; revisit if the reproduction shows starvation between models); per-model admission pools (loses the process-wide bound).

### D4 — No run holds the whole budget while work is pending

For a single-model deployment the model's share equals the process budget, so its run width equals capacity and admission is exclusive. The layout and admission rules SHALL together ensure that, when the budget admits ≥ 2 runs, a second run remains admissible (share capped below the budget, or the batch charged as more than one unit when it occupies more than one window's worth of work). Which of the two, the reproduction decides.

Alternatives: leave it, and rely on the batcher coalescing (exactly the configuration that queues single-file when batching is off or the determinism probe degrades).

### D5 — Concurrency parity with pre3 is a layout property, not an accident

Under the CPU policy, a model with a share ≥ 2 SHALL keep at least two concurrently admissible runs (two sessions, or concurrent runs on one session where the code already treats a session as safe to share, as `permit.Pool`'s capacity-over-items behavior does for the script path). A share split that would floor a model to one thread per model while the process holds headroom is reported and reclaimed. Group 1 measured the layout side: pool mode calibrates to 8 sessions × 1 thread (1036.7 req/s at c128), while the uncalibrated policy default is 1 session × 8 threads (379.5 req/s) — so the concurrency comes from calibration plus D4's width cap, not from a fixed session count.

**ORT spinning: the policy's forced `false` stays.** The original design here was to restore ORT's spinning default for wide or batched sessions. Group 1 measured both directions at a fixed 4 × 3 layout and **spinning off is faster in both revisions** (pre3: 879.0 vs 728.3 req/s at c128, probe p50 138.0 vs 166.9 ms; pre4: 405.7 vs 335.2). The clause is dropped and the default kept, with the qualified-host differential as the tie-breaker if that host disagrees. An explicit `allow_spinning` remains authoritative.

Alternatives: `autoTuneWorkers` unchanged from pre3 with the gate added (restores concurrency but not the process bound — the pre4 change exists to add that bound); one session with intra-op width = whole budget (status quo; cannot overlap a long batch with anything, and measured to pin `cpu_peak_tokens` at capacity).

### D6 — Calibration is never on the request path, and readiness means it

A model is ready only after its layout is decided. With `preload` unset, a request SHALL receive an explicit not-ready/busy answer instead of triggering and blocking on calibration, and concurrent requests SHALL NOT wait on another request's initialization. The alternative — default `preload: true` — is preferred where the deployment can afford boot time, and is decided per deployment, but readiness SHALL NOT depend on it.

Alternatives: leave the current lazy `entry.once` path (one cold request decides everyone's fate); preload everything unconditionally (boot cost for models that are never queried).

### D7 — A truncated trial is unresolved, not "the smallest layout won"

`calibrateLayout` checks its deadline only between candidates (`calibration.go:190`) and orders the serial baseline first, so a truncated search silently selects the narrowest layout. The deadline is checked within a candidate too, and any trial that ends without a measured winner records `unresolved` with its reason (`deadline`, `baseline_failed`, `cpu_accounting_unavailable`, `p95_regression`, `no_valid_candidate`), distinguishable from a calibrated result in the log and in `EMB.INFO`.

Alternatives: keep the 4 s bound and accept the smallest layout (that is the defect); make the deadline unbounded (boot becomes unpredictable; the spec already allows an in-flight call to overrun).

### D8 — Attribution is part of the fix, not a follow-up

Admission wait is exported from the wrapper that holds the gate; queue depth (controller waiters, batcher channels, pool requests) becomes a gauge; busy refusals are counted; and the live server reads cgroup throttling so "CPU starved" can be separated from "slow ONNX". The existing `emb.dispatch.wait` is only the channel hand-off and `emb.inference.duration` spans tokenize + admission + ORT, so neither answers the incident's question today.

Alternatives: attribute from outside (a profiler/`perf` on the host) — not available during a rollback, which is when the answer is needed.

### D9 — The gate asserts the class, in three tiers

Tier 1 (deterministic, model-free): deadline-bounded acquire, admit-if-fits while a wider head is blocked, release on error/panic, layout concurrency over a 1–4-model registry at a 3-CPU budget. Tier 2 (in-process, in `test-cgo`, no download): a plain-`EMB` burst against an in-repo fixture asserting a stated p99 bound, a bounded queue with refusals, a reported admission wait, and drain after disconnect. Tier 3 (host-qualified): the frozen pair, same host, same model/config/load, diffed through the existing bundle/fingerprint rules, recording latency percentiles, admission wait, queue depth and the effective layout.

Self-verification follows the repo's existing mutant pattern: no-deadline acquire, unbounded queue, single-single-threaded layout, calibration reachable from a request, cache hits counted as traffic — each must turn the gate red and name its assertion. The gate asserts *these properties*, not one privileged cause, so it cannot be satisfied by pinning a layout.

Alternatives: rely on `bench/fargate` cells (measures throughput on a matrix, not admission latency, and needs Docker and a full tier run per change); a single nightly differential (a day of exposure per regression, and it cannot run per change).

### D10 — The runtime correction is evidence-gated

Groups 2–5 build the bounded path and make the failure attributable; which of them is the *minimal* correction follows the reproduction, using the conditional-solutions table in `proposal.md`. No speculative scheduling, timeout or retry change ships.

## Risks / Trade-offs

- **A deadline can refuse work that used to eventually succeed.** Trading a deep queue for bounded errors is the intent, but the bound and the queue size must come from the reproduction's offered load, and the gate's burst test fixes them so they cannot drift silently.
- **Borrowing can oversubscribe.** The reserved-slot scheme admits narrow runs beside a wide one; the resident ceiling (share × width) keeps the total inside the process budget, and the existing oversubscription warning stays as the backstop.
- **Concurrent runs on one session change batch composition.** If D5 takes the shared-session route, the batcher and the calibration trials must agree on what a run is worth; the reproduction's per-cell numbers decide between shared sessions and two sessions.
- **Finite default for `max_concurrent_requests` is a behavior change.** It is documented, config-checked, and covered by the compatibility tests; deployments that set it explicitly are unaffected.
- **The frozen pair may not separate on every host.** A shared CI runner may not expose the stall; then the differential reports `inconclusive` with the host property and the in-process tier carries the enforcement, rather than claiming a verdict.
- **Extra latency from cancellation checks.** Bounded and equal between arms; the gate compares revisions on the same host, so instrumentation cost cancels.

## Migration Plan

1. Group 1 reproduces and freezes the pair; it selects the minimal correction and states which measurement was unavailable if it cannot.
2. Groups 2–3 (admission, then layout) land the bounded path; group 4 makes startup honest; group 5 makes it attributable. Each lands with its own tests and keeps `cpu_budget: off` restoring the legacy policy.
3. Group 6–7 build the gate and its mutants; group 8 wires it into `test-cgo` and the qualified reference run; group 9 records the differential result.
4. Rollback is per group: `cpu_budget: off` restores legacy sizing and admission, so a deployment can return to pre4 behavior without a rebuild.

## Open Questions

- Whether the deployed path takes the batched or the degraded-unbatched branch (batching `timeout`, and whether the determinism probe fails on the production model) — this selects D4/D5's concrete form.
- Whether concurrent `Run` calls on one ONNX session are acceptable for the production model (correctness and throughput), or whether two half-width sessions are required.
- The finite default for the admission queue and whether the outer `max_concurrent_requests` should default finite in the same change or be left to the operator.
- Whether the differential belongs on the free `ubuntu-24.04-arm` runner or a dedicated-vCPU host, which the `bench-host-check` verdict for the stall's reproduce-ability decides.
