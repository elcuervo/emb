# Proposal

## Why

After `emb` pre4 was deployed to `api-fargate`, Datadog recorded an `/search/photos` request spending about 10 seconds waiting for an `Emb::Proxy` response through `E5.encode`, then returning HTTP 500; web requests in the same window timed out, and the API was rolled back. This ties the incident to the emb-backed request path, but not yet to a specific server mechanism: CPU starvation, incorrect capacity planning, a stuck inference request, or deployment/configuration mismatch remain possible. The goal is to reproduce the production path and prevent a same-class latency regression from shipping again, without treating an unverified cause as fact.

The operator confirms **pre3 works correctly**; pre4 is the failing release. Use the repository's `v0.4.3.pre3` (`a0ffc5be4661e10b7bb54f3b24f482a69c819f91`) and `v0.4.3.pre4` (`0ab054edb938aa4bdbac7236385a81187e386547`) peeled commits as the comparison boundary, and verify the deployed package/binary identities against them. Do not substitute current HEAD against itself.

Read from the tree since the handoff was written, the relevant pre4 delta is small and structural: `git diff a0ffc5b..0ab054e -- internal/pipeline internal/permit/pool.go internal/permit/autotune.go` is empty, so the runtime change is one new object — a process-wide CPU admission gate — plus the layout and startup policy built around it. That gate waits **without a deadline, without cancellation and without a queue bound**, it is **strictly FIFO at the head**, and pre4 simultaneously reduced per-model concurrency to a single session. A slow request therefore no longer overlaps with its peers; it parks every later request behind it, forever, until the run it waits on completes. Nothing in the repository would fail if that shipped again.

## What the code shows (pre4 = the failing release)

Cause is **not** established. These are the mechanisms identified in the source; which one fired is the subject of group 1 of `tasks.md`, and the discriminator for each is recorded there.

| Mechanism | Evidence (paths from pre4; the CPU path is unchanged to HEAD) |
|---|---|
| One process-wide admission gate, on by default | `internal/permit/budget.go` (new); `registry.go:330` `permit.NewBudget(plan.Budget)`; every embed/script/image run charges it — `cpu_budget.go:123,135`, `registry.go:711`, `image.go:259` |
| Admission waits are unbounded and uncancellable | `budget.go:29-52`: `sync.Cond` wait, no context, no deadline, no queue limit; `defer release()` is the only release path |
| Strict FIFO head-only admission blocks idle capacity | `budget.go:36` (`b.waiters[0] != w || !b.canAdmit(width)`); an oversized head parks every follower until the gate drains (`:54-59`) |
| Per-model concurrency collapsed to one session | `registry.go:560-567` (`numWorkers = 1` under the policy; pre3: `autoTuneWorkers`); with batching off or determinism-degraded (`registry.go:665-677`) that is one session serving one request at a time |
| Layout can silently collapse to 1 thread | calibration falls back to `{Workers:1, Threads:max(1,configuredThreads)}` = 1×1 (`calibration.go:134,198-215`); per-model share `max(1, floor(...))` does not reclaim when models exceed budget (`cpu_budget.go:87-102`) |
| Startup calibration is on the request path for a lazily loaded model | if `preload` is unset, the first request runs `ensurePool` + `calibrateLayout` inside `entry.once` (`registry.go:1203-1205`), and `EMB.READY` was already true (`cmd/emb/main.go:168`); trials hold the same gate (`registry.go:695`) |
| Readiness does not depend on a usable layout | `SetReady` is gated only on `modelCount > 0` (`cmd/emb/main.go:168`) |
| Observability cannot attribute the stall | `Budget.Acquire` times nothing; `RunUs` spans tokenize + admission + ORT (`pipeline/pool.go:62-68`); no queue-depth gauge; `ERR busy` is not counted; the server never reads cgroup throttling |
| Amplifier, unchanged but now decisive | no server-side request deadline anywhere (`server.go:363-408`); `max_concurrent_requests` defaults to `0` = unlimited (`config.go:46`); client `read_timeout = 10` (`gems/emb/lib/emb/configuration.rb:53-54`) explains the observed ~10 s figure |

Two of these are already required by `openspec/specs/inference-cpu-budget/spec.md` and are simply unimplemented: *"Idle model capacity SHALL be borrowable"* (contradicted by the FIFO head rule) and *"The controller SHALL report admission wait separately from preprocessing, local pool wait, and run time"* (`Budget` reports nothing).

## What Changes

1. **Bounded, cancellable admission.** Acquire with a deadline and a context threaded from the command handler; bound the queue and answer `ERR busy` past it instead of queueing without limit; abandon admission and stop publishing results when the client has gone away. Overload becomes a bounded error, never an unbounded wait.
2. **Fair, work-conserving admission.** Idle capacity is borrowable as the existing spec requires: a narrower waiter is admitted when its width fits even if a wider head is still blocked, and no single run holds the whole budget for the common single-model case.
3. **Layout parity with pre3.** A process with budget ≥ 2 keeps at least two in-flight runs available when demand exists; the 1×1 fallback and the per-model share split are reported rather than silent; ORT spinning is not disabled under the policy; a share is reclaimed when models exceed the budget.
4. **Honest startup.** Readiness requires a decided, usable layout; calibration never runs inside a request (`preload` default, or an explicit not-ready answer); a deadline-truncated calibration is recorded as unresolved, not as "the smallest layout won".
5. **Attribution.** Admission wait is measured separately from pool wait and run time (spec-mandated), queue depth is visible, `ERR busy` is counted, and the live server reads cgroup throttling so CPU starvation can be told from slow ONNX.
6. **The regression case — the deliverable that prevents recurrence.** A three-tier gate under `bench/` and `internal/`, plus a host-qualified pre3-vs-pre4 differential that must be **red on pre4**, **green on pre3**, and **green on the fix**:
   - *Tier 1, deterministic and model-free* (`internal/permit`, `internal/registry`): admission with a deadline returns a bounded error under saturation; a narrow waiter is admitted while a wider head is blocked; permits release on error and panic; layout assertions over 1–4-model registries at a 3-CPU budget (no silent 1-thread-per-model, sessions ≥ 2 under demand); calibration is not reachable from the request path.
   - *Tier 2, in-process behavioural gate* (the `EMB_BENCH_SMOKE`/`test-cgo` pattern, extended from the scripted path to **plain `EMB`**): a burst of concurrent requests asserts a stated p99 bound, a bounded queue with `ERR busy` past the cap, a reported non-zero admission wait, and that a disconnected client's work stops. Planted regressions must trip it: no-deadline acquire, `numWorkers=1` with width = budget, calibration on the lazy path, unbounded queue.
   - *Tier 3, host-qualified differential* (`just bench-host-check` + the evidence bundle/diff built in `performance-insight-and-smoke-gate`): the same host, model set and config, `a0ffc5b` vs `0ab054e`, recording latency percentiles, admission wait, layout and the CPU fraction per arm. It refuses incomparable fingerprints and marks unmet preconditions `invalid`, so a green run cannot mean "the assertion never ran".
   - *Routine enforcement*: tier 1 on every change with the existing Go suite; tier 2 in `test-cgo`; tier 3 on the qualified runner per release. `bench/smoke/verify-gate.sh` is extended to prove the new gate fails.
7. **Keep the external contract stable.** No RESP wire-format, model output, or Ruby-client API change. No new required configuration key; any default that changes (e.g. backpressure) is explicit, documented and covered by the existing config checks.

## Capabilities

### New Capabilities

None. The durable behavior belongs in the existing inference performance and CPU-budget capabilities.

### Modified Capabilities

- `inference-performance`: require that a plain embedding `EMB` stays within a bounded latency under representative concurrency and overload, that the selected layout does not silently collapse below the process concurrency, and that the layout/admission regression gate exists, is proven to fail on planted regressions, and is enforced routinely.
- `inference-cpu-budget`: require admission to be bounded and cancellable rather than unbounded FIFO, to keep idle capacity borrowable, to never let one run monopolize the budget while work is pending, to record a truncated calibration as unresolved, and to report admission wait separately as the capability already states.

## Impact

- **Server/runtime:** `internal/permit/budget.go` (deadline, queue bound, borrowable admission, wait accounting); `internal/registry/cpu_budget.go` (share/layout floor, wrapper accounting); `internal/registry/calibration.go` (deadline semantics, off the request path); `internal/registry/registry.go` (layout defaults, readiness); `internal/server/` (`context` through `handleEMB`/`embedTexts`, `ERR busy` accounting, `INFO`/`STATS` surfacing); `internal/pipeline/` (only if the batch-cost/backpressure decision requires it).
- **Regression tooling:** `internal/permit`, `internal/registry`, `internal/server` tests; `bench/smoke/` mutants and `verify-gate.sh`; a new plain-`EMB` gate fixture beside `testdata/laya`; `bench/script/` and `bench/fargate/` for the differential arm; `justfile`; `.github/workflows/ci.yml`.
- **Contracts/docs:** delta specs for `inference-performance` and `inference-cpu-budget`; `docs/performance-validation.md` and `BENCHMARK.md`; `docs/configuration.md` / `docs/operations.md` for any default and stat change.
- **Evidence dependency:** the deployed configuration, model key, task CPU/memory limits, `GOMAXPROCS`, lazy mode, model count, cache state and incident-window server logs remain needed to *select* the minimal fix and to bind the reproduction. They do not block building the gate: the gate asserts the class of failure, which is visible in the frozen pre3/pre4 pair alone.
- **Cost:** none recurring beyond the existing free `ubuntu-24.04-arm` runner and the `test-cgo` budget; the tier-2 gate must stay inside the job's existing wall-clock.

## Validation

The change is done when the gate is shown to be falsifiable and the frozen pair separates:

```bash
nix develop
just bench-host-check                     # qualified host, failing property named
EMB_BENCH_SMOKE=1 go test ./internal/server/ -run PlainEMB -count=1   # tier 2
bash bench/smoke/verify-gate.sh           # every planted regression red, clean tree green
just bench-plain-emb BASE=<pre3> CAND=<pre4>   # tier 3: pre4 red, pre3 green
just bench-plain-emb BASE=<pre3> CAND=<fixed>  # tier 3: green, within tolerance
```

Acceptance:

- Tier 3 separates the frozen pair on the qualified host: pre4 exceeds the plain-`EMB` latency bound or fails the admission-wait/queue-depth assertions while pre3 passes the same run with identical configuration, model and load.
- Every planted regression in tier 2's mutant set turns the gate red and names the assertion it broke; the unmodified tree passes.
- The reproduction (group 1) names the mechanism that fired, or reports the pair as not separated and says which measurement was unavailable — never a fabricated verdict.
- Existing gates are unchanged: `EMB_BENCH_BUDGETS=1`, the golden baselines, the scripted smoke gate, `just lint`, `just deadcode`, `just cover`.

## Non-goals

- **No speculative runtime change.** The fix follows the reproduction; mechanisms that the frozen pair cannot separate are recorded, not patched.
- **No new configuration keys** and no new telemetry dependency; existing `EMB.STATS`/`EMB.INFO` state and the optional OTLP metrics carry the new signals.
- **No timeout increase** of the client, the server or the health path as a fix; a longer diagnostic deadline must report whether the backlog drains.
- **No change to `EffectiveNumCPU()`'s sub-one-CPU flooring** (the Fly `shared-cpu-*` collapse), which is a separate change; this one may report it but does not fix it.
- **No embedding, wire-format or gem API change**; replies stay byte-identical.
- **Not a substitute for the scripted-path smoke gate** added by `performance-insight-and-smoke-gate`; that gate covers `EMB.EVSHA` on `testdata/laya`, not the plain embedding request in this incident.

## Open questions for the operator

These select the fix and bind the reproduction; each is a fact only the deployment can supply, and none of them blocks building the gate:

1. The deployed model map and whether `Emb[:hyperclusters]` resolves to the `e5` entry; the effective config file and its `preload`, `batching`, `workers`, `intra_op_threads`, `cpu_budget` settings.
2. Task size, `GOMAXPROCS` (env or image), cgroup quota/affinity as seen *inside* the container, and replica count.
3. The application's `lazy:` mode, pool size, request timeout/retries, and how many models one `/search/photos` request touches.
4. Whether the deploy coincided with a cold cache and a cold model, and the boot/`EMB.STATS` logs from the incident window.

## Investigation Findings

Evidence below comes from the release history and inspected source. Mechanisms are confirmed code behavior; their involvement in the outage remains **[INFERENCE]** until reproduced with the incident workload.

| Priority | Clue and evidence | Discriminating experiment |
|---|---|---|
| 1 | Pre4 introduces process CPU planning and calibration (`dea1d1f`, `dceb3f1`, `3d14acf`, `abc3b61`, merge `837ca5c`). `internal/registry/registry.go:679-715` calibrates unset dimensions and wraps served sessions in the shared governor. The same configuration can therefore select different session/thread counts than pre3. | Hold the client fixed; compare pre3, pre4 defaults, pre4 `cpu_budget: off`, and pre4 with the measured pre3 layout explicitly pinned. Record effective layouts, not just YAML. |
| 1 | Calibration is on model initialization; `calibration.go:58-60` explicitly permits an in-flight call to exceed its nominal four-second deadline. Candidate creation, warmup and repeated inference add real work; trials use at most four rows / 256 tokens (`:67-75`), not arbitrary production batches. | Separate cold first request, concurrent cold first requests, and warmed steady state. Compare calibration enabled/disabled and preload separately. Record candidate scores, fallback reasons and initialization wall time. |
| 1 | `internal/permit/budget.go:29-58` uses strict FIFO weighted admission without a cancellation/deadline parameter. A wide head waiter prevents smaller followers from entering; oversized runs must execute alone. `registry/cpu_budget.go:123-126` waits inside the session wrapper. | Drive one long/wide model concurrently with short `hyperclusters` requests; capture queue positions, widths, admission timestamps, active tokens and actual ONNX duration. Distinguish intended fairness from sustained starvation or lost releases. |
| 1 | Pre4 pins the embedding layout to a single session and, with batching off or determinism-degraded, to a single worker (`registry.go:560-567`, `:665-677`); pre3 sized `autoTuneWorkers` sessions. A burst then queues single-file where pre3 overlapped. | Measure the same burst under `batching.timeout: 0` and default batching, with the determinism probe forced to fail, on both releases. |
| 2 | Pre4 changes deferred `:batch` grouping to per-model shares (`eb00116`, `10b6b2d`). But pre3 already has plain-EMB single-model shares and bounded parallel dispatch: this is **not** a newly introduced parallelism mechanism. Grouping can change how much work one command holds and how models contend. | Cross old/new client and server versions. Replay the actual lazy mode and model mix; record command names, text counts and overlapping shares. If only emb-server changed in production, demote this hypothesis. |
| 2 | Unbatched `Worker.process` runs each text serially (`internal/pipeline/pool.go:74-93`). Pool/batcher requests and result waits have no context deadline. A large command can monopolize a worker; a client timing out does not cancel this work. This is an amplifier, not by itself a demonstrated pre4 change. | Sweep observed texts-per-command and token lengths, including a large request followed by short requests. After clients time out, measure outstanding work and recovery without restarting the process. |
| 3 | Ten-second read/write timeouts and the blocking `RoundRobinPool` are unchanged between pre3 and pre4. Pool checkout can consume web-thread time before socket timeout begins. The observed ten seconds is compatible with the read timeout, not proof of ten seconds of ONNX compute. | Capture the actual exception/cause and split pool checkout, connect/write, read and application time. Count wire sends and pending batch items; do not infer retries merely from the timeout. |
| 3 | Pre4 adds latency histogram/export work (`568ce21`). `internal/telemetry/histogram.go:80-88` uses a tracked-name lookup and atomic counters; this alone does not establish a ten-second stall. | Only after higher-priority isolation, compare export enabled/disabled and capture lock/CPU profiles. Keep instrumentation identical between measured arms. |

### Measurement blind spot

Do not assume the current metrics already separate admission wait from inference. `budgetedSession.Run` calls `Acquire` without recording its wait. The worker's `totalLat` spans the complete `process` call (`pipeline/pool.go:62-68`), including preprocessing, admission and inference; `Pool.Stats().RunUs` exports that aggregate. `dispatchWaitUs` measures the channel-send handoff; for the buffered batcher it does not include all residence time after enqueue. Consequently, a high `emb.inference.duration` cannot by itself distinguish CPU starvation, governor waiting or slow ONNX execution. Group 5 of `tasks.md` closes this before the differential is required to be conclusive.

### Existing evidence and its limits

- `config-prod.yaml` names `e5`, points at a local `hyperclusters/model.onnx`, and selects `pooled_sentence_embeddings_debiased_normalized`; its model paths are workstation paths. It is not proof of the deployed configuration. Resolve `Emb[:hyperclusters]` against the actual model map. A missing model should produce an immediate error and is a poor explanation for sustained ten-second stalls.
- The local `bench-reference-local` scaffold exercises native GLiNER2 shape/A-B and a separate Docker MiniLM matrix. The earlier run produced 84 cells and a passing self-diff, but `host-contract` failed on Darwin. Its bundle does not contain the GLiNER2 behavior assertions. It neither reproduces the production E5 path nor establishes pre3/pre4 parity, and a self-diff measures serialization/comparison plumbing, not the noise floor.
- Executed the existing timeout diagnostic inside `nix develop`: `cd gems/emb && bundle exec ruby ../../bench/repro/client-timeout/repro.rb`. Explicit one-second timeout plus three reconnect attempts produced four sends (24 items); failed pending items were cleared and re-forcing sent nothing. The ten-second/default-retry arm sent six items once. This verifies the current mock-backed guard, not incident causality or the deployed client's settings.
- The scripted-path smoke gate added by `performance-insight-and-smoke-gate` is not a substitute: it drives `EMB.EVSHA` against `testdata/laya`, so it cannot see a plain-`EMB` admission stall.

## Ordered Investigation and Implementation Handoff

Groups 1–8 are executable as `tasks.md`; group 1 is the reproduction that selects the fix, and groups 2–9 build the bounded path and the gate.

1. **Freeze the incident identity and workload.** Preserve the failing trace, exception chain, deploy/rollback times, exact pre3/pre4 server and client gem versions, server binary/image digest, ONNX Runtime version, model/tokenizer/external-data digests and effective config. Record task CPU/memory limits, cgroup ancestry, affinity, `GOMAXPROCS`, replicas, application worker/thread counts, pool size, lazy mode, request timeout and retries. Capture sanitized text lengths, texts per command, model mix, cache state and arrival concurrency. Treat pre3's working status and the reported outage as given; collect details to match the reproduction, not to re-confirm the report.
2. **Build an incident-specific local reproduction, not another scripted benchmark.** Build both tagged servers in isolated worktrees with the same toolchain/runtime and read-only copies of the actual E5/hyperclusters model. First replay direct RESP `EMB` using the deployed model key/output settings; then run the Ruby `E5.encode`/Proxy equivalent with the actual lazy mode. Preserve the same request corpus, output checks and offered load. Local macOS or a throttled Linux container may reproduce mechanisms, but label it diagnostic; a production-shaped ARM64 Linux run is required for reference numbers.
3. **Separate changed components.** Run the four combinations of pre3/pre4 server and pre3/pre4 client with identical config and workload, plus direct RESP against both servers. Pin package dependencies instead of accidentally loading the working-tree gem. If deployment changed only the server, prioritize that arm while retaining the client cross-check. A regression in direct RESP implicates the server/config path; one confined to the changed client implicates dispatch/pooling; an interaction requires preserving both in the regression case.
4. **Separate initialization from capacity collapse.** For each arm capture process start → ready → first valid embedding and concurrent first-use requests, followed by warmed serial, representative concurrency, overload and recovery phases. Preserve actual batching/determinism behavior. Repeat short/typical/long and mixed-length workloads, single/multiple models, cache misses and controlled cache hits. Include the production traffic distribution; do not rely only on closed-loop clients that stop offering load while stalled. Observe web-thread/pool occupancy alongside server queues so application-wide failure is explained.
5. **Attribute elapsed time before tuning.** Record client checkout/connect/write/read time; server receive, model initialization, tokenize, enqueue/dequeue, governor acquire/release, underlying ONNX entry/exit and reply completion. Collect CPU user/system, quota throttling/steal, RSS, goroutine/thread counts, selected sessions/threads, active/queued admission and errors. Use existing counters plus temporary diagnostic instrumentation where the blind spot above demands it. Check token release on errors and whether abandoned requests drain after a timeout. Keep trace sampling bounded and equal between arms.
6. **Run one-factor policy experiments.** Compare pre4 defaults to `cpu_budget: off`, separately `calibrate: false`, then explicitly pinned pre3 workers/threads and preload. Record all settings each switch changes: `cpu_budget: off` changes calibration, admission and legacy defaults together, so improvement localizes a policy family, not an individual cause. Sweep only the demonstrated quota/share/layout boundary and relevant batch sizes. Include unequal model widths for FIFO head-of-line blocking and production-sized batches for calibration misranking. Do not raise timeouts as a fix; a diagnostic longer deadline must report whether work eventually completes and how the backlog behaves.
7. **Bisect the failing component.** Once a deterministic or statistically repeatable criterion separates pre3 and pre4, bisect that exact interval with fixed artifacts, workload and resources. Start with the CPU-policy commits above if direct RESP regresses; inspect `eb00116`/`10b6b2d` if client grouping is required; retain telemetry as a lower-priority branch. Repeat noisy points and report inconclusive points rather than assigning false good/bad verdicts. Keep unrelated working-tree changes out of the comparison.
8. **Implement only the demonstrated correction.** Follow the conditional solutions below. Prove the isolated culprit reproduces the slowdown and that reverting/correcting it restores service under the same load. Keep embedding output and error semantics intact; avoid speculative scheduling or retry changes.
9. **Leave a reproducible regression gate and evidence bundle.** Turn the minimized failure into deterministic coverage where possible and a real-model plain-EMB benchmark for the resource-sensitive portion. Record revisions, hashes, host, effective configs, repeated interleaved independent measurements, latency tails, throughput, errors/timeouts, backlog/recovery and assertion outcomes together. Compare pre3 versus fixed pre4 on the same qualified host; refuse incomparable runs and report unmet preconditions as invalid. Verify the gate fails with the culprit restored, not just that it passes the fix.

## Conditional Solutions for the Implementation Agent

| Demonstrated cause | Smallest corrective direction | Required proof |
|---|---|---|
| Cold calibration blocks live traffic past its deadline | Move required initialization before readiness for the deployed path, or bound calibration work between trials and use a validated safe layout. Do not close sessions while CGo calls are running. | Cold/concurrent first use stays within the agreed request/startup budget; readiness cannot expose an unready model; warm capacity is preserved. |
| Calibrator selects a low-capacity layout | Correct trial shape/scoring or the demonstrated candidate/fallback defect; use production-representative batching and lengths. Explicit layout pinning is a diagnostic/operational mitigation, not evidence that automatic selection is correct. | Automatic selection meets the pre3-relative latency/throughput contract at the failing boundary without excess memory/threads; explicit overrides still hold. |
| Shared-budget under-sizing or FIFO blocking collapses concurrency | Correct allowance/share accounting or admission scheduling only where measured. Preserve process bounds and fair progress; borrowing cannot exceed resident capacity. | Mixed-model short requests progress during long work, oversized requests still finish, and permits release on errors. No idle-core claim without measured CPU availability. |
| Timed-out queued work sustains the outage | Propagate abandonment/deadlines through the demonstrated queue boundary and prevent obsolete queued work from entering inference where safely cancellable. In-flight ONNX cancellation needs a separate safety check. | Outstanding work drains, later requests recover without restart, no permit/session leaks, and no use-after-close. |
| Client grouping or pool contention triggers the failure | Correct the responsible share construction/concurrency or checkout behavior while retaining per-item results and intended failure isolation. Do not add retries or silently convert errors into empty embeddings. | Cross-version matrix isolates the client change; wire command counts and per-request text counts match expectations; web threads recover after errors. |
| Deployment/config/model mismatch | Fix the packaging or effective configuration and add a pre-traffic embedding/readiness check for the exact model key and output. | The deployed artifact/config is identified and the same model request succeeds before traffic; no unrelated runtime changes. |

## Completion Criteria and Remaining Evidence

- The implementation report names the first bad change or a minimized causal interaction, links measurements that distinguish it from the other hypotheses, and explains both the slow embedding and the wider web-request failures.
- The regression test/experiment is failing-before and passing-after. Independent repeated pre3/fixed runs establish latency, throughput, error-rate and recovery thresholds from the supplied workload/SLO; a self-diff or arbitrary relaxed timeout is not acceptance.
- Every enforced assertion has an observed outcome. Missing production artifacts or unavailable host capabilities remain explicit blockers for production-equivalent claims, not reasons to fabricate a baseline.
- Known now: pre3 works, pre4 caused the reported outage, the release commit boundary, the CPU-path delta above, and the concrete policy/client changes. Still to bind to the reproduction: deployed server/client hashes and effective config, exact model/corpus/request shape, resource limits and incident-side timing/exception data. Continue all local differential work with recorded assumptions while acquiring these; do not ask the operator to prove the incident again.
- The scripted-path change `performance-insight-and-smoke-gate` is complete and archived separately; it is not a substitute for this regression gate.
