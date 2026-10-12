# Tasks

## 1. Host contract and preflight

- [x] 1.1 Add a host probe under `bench/` that reports architecture, effective CPU allowance and its source, memory limit, and sampled steal and throttle deltas. Reuse the server's own resolution (`registry.EffectiveCPUAllowance`) rather than a second cgroup parser, so the report and the server cannot disagree. Verify with tests over synthetic cgroup layouts: a sub-one-CPU quota reports unqualified and names the quota, a quota equal to the presented CPUs reports qualified, and a missing cgroup file reports the scheduler/affinity source without inventing a quota.
- [x] 1.2 Add the `just bench-host-check` target and print the verdict plus the failed property. Verify by running it on this machine (unqualified: the platform is not the deployment target and steal is unmeasurable) and recording that output in the docs.
- [x] 1.3 Document the contract and the host ladder in `docs/performance-validation.md`: which hosts qualify, the cost of each rung (free `ubuntu-24.04-arm`, Hetzner CAX21/CCX13, Fargate ARM 4 vCPU/8 GB, and why Fly `shared-cpu-*` is excluded), and the rule that a run on an unqualified host is not evidence. Verify the documented `just bench-host-check` invocation is the one the target defines.

## 2. Evidence bundles and comparability

- [x] 2.1 Extend the `host` block `bench/fargate/main.go` already emits into the fingerprint (architecture, effective CPU allowance and source, steal and throttle verdict), and add revision, model digests, configuration, per-cell metrics, and an assertion list with `pass`/`fail`/`invalid`/`skipped` outcomes. Verify with a test that a bundle round-trips and that every field the diff reads is present.
- [x] 2.2 Make baseline capture refuse an unqualified host (`-mode baseline`) and make the diff (`-mode diff`) refuse two bundles whose fingerprints differ, printing both. Verify with fixture bundles: differing fingerprints are refused, matching fingerprints produce per-cell verdicts within the existing tolerances (±5% req/s, ±10% p50).
- [x] 2.3 Update `bench/fargate/README.md` and the `docs/performance-validation.md` bundle section with the fingerprint requirement and the refusal rules. Verify the README's documented commands match the flags the harness accepts (`go run ./bench/fargate -h`).

## 3. Smoke gate: fixture and behavioural assertions

- [x] 3.1 Add the smoke fixture preset beside `testdata/laya` (fixed inputs, no ARGV, deterministic outputs) and confirm it is outside the stamped site presets. Verify `python3 website/tools/stamp-presets.py --check` still passes and no `scripts/*.lua` digest changed.
- [x] 3.2 Add the gate test in `internal/server` behind `EMB_BENCH_SMOKE=1`, reusing the in-process server helper the budget tests use, driving serial → saturating burst → serial against the fixture and asserting the class sequence, the allowance halving after the sustained quiet window and returning to the cap on the first `throughput` window, the cap and floor, and that layout (session and thread counts) does not change across adaptation. Verify `EMB_BENCH_SMOKE=1 go test ./internal/server/ -run Smoke -count=1` passes inside the `test-cgo` wall-clock budget.
- [x] 3.3 Add the kill-switch, profile-pinning, cache-idle and reply-identity assertions to the gate: `autotune: off` holds the configured concurrency, `capacity: latency|throughput` pin their layouts with the reported autotune state, a cache-warm window classifies `idle` with frozen run counters, and replies are byte-identical across arms. Verify each assertion with its own subtest run under `EMB_BENCH_SMOKE=1`.
- [x] 3.4 Add the reachability precondition and the honest skip: record the CPU fraction the saturating phase achieved, report the saturation assertion as skipped with the host property when it was not reached, and never report a pass for it. Verify by forcing an unreachable phase (a single-core budget on a many-core host) and asserting the gate reports a skip, not a pass.
- [x] 3.5 Add the `just bench-smoke` target and document the gate — what it asserts, its cost, and the failed-property report — in `docs/performance-validation.md`. Verify `just bench-smoke` runs the gate and prints the assertion outcomes.

## 4. Smoke gate: relative throughput arm

- [x] 4.1 Add the fixed-concurrency control arm (`autotune: off` at the same cap) and gate the adaptive arm's throughput against it in the same run, medians over repeated rounds, warmup excluded, tolerance recorded beside the constant with its reason. Verify the gate passes on the unmodified tree and that the tolerance math is unit-tested against a stubbed measurement source (a within-tolerance pair passes, a beyond-tolerance pair fails).
- [x] 4.2 Partition the load generator from the server's cores where the host has enough of them, reusing the existing `app_cpus`/`bench_cpus` knobs, and pin them in the gate. Verify the gate reports the partition it used and still passes with it applied.

## 5. Gate self-verification

- [x] 5.1 Add `bench/smoke/verify-gate.sh` with committed patches under `bench/smoke/mutants/` that plant the four regressions (inverted allowance dwell, saturation threshold out of reach, reply-cache hits counted as traffic, runtime layout reshape), applied in a detached worktree so the working tree is never modified. Verify the script exits non-zero when a mutant passes and names the assertion each mutant breaks.
- [x] 5.2 Run the sweep and record the result (each mutant red, clean tree green) in `docs/performance-validation.md`. Verify the recorded output matches a fresh `bash bench/smoke/verify-gate.sh` run.

## 6. CI wiring

- [x] 6.1 Enable `EMB_BENCH_SMOKE=1` in the `test-cgo` job and assert the gate ran rather than skipping, inside the job's existing 1200 s budget (gate ceiling ~120 s). Verify a CI run whose log shows the gate's assertion outcomes and a suite wall-clock inside the budget.
- [x] 6.2 Confirm the gate leaves `scripts/coverage.sh gate` and `just deadcode` passing, and that no new production code was added outside `bench/` and `internal/server` tests. Verify with the local runs of both checks in `nix develop`.

## 7. ARM64 reference tier

- [ ] 7.1 Add a `workflow_dispatch` job on `ubuntu-24.04-arm` that runs `bench-host-check`, then the shape and A/B runs against the scripted model set cached by `actions/cache` (mirroring the `models/minilm` cache in `test-cgo`), and uploads the bundle as an artifact. Verify a dispatched run produces a qualified bundle and that the job is not a required check.
- [ ] 7.2 If the runner reports unqualified, pin the reference tier to the documented qualified host (rentable dedicated vCPU or the Fargate ARM task, driver inside the task under a `taskset` partition) and record the verdict and the chosen host in `docs/performance-validation.md`. Verify the recorded verdict is the one `bench-host-check` printed.
- [ ] 7.3 Record the first host-qualified baseline and diff it against itself through `just bench-fargate-diff` / the shape harness to establish the noise floor. Verify the same-host diff passes within tolerance and a cross-host diff is refused.
- [x] 7.4 Add the cross-host rule to `BENCHMARK.md` (absolute numbers are never comparable across hosts; gates compare a host against itself) and point at the host ladder. Verify the rule is stated where the existing benchmark tables are introduced.

## 8. Integration verification

- [x] 8.1 Run the whole documented loop in `nix develop`: `just bench-host-check`, `just bench-smoke`, `bash bench/smoke/verify-gate.sh`, `EMB_BENCH_BUDGETS=1` budgets, `just test`, `just lint`, `just deadcode`, `just cover`. Verify the existing budget gates and golden baselines are unchanged and all listed commands pass.
- [ ] 8.2 Verify the reference path end to end on the qualified host: host check → shape run → A/B run → bundle → same-host diff, with the autotune transitions reproduced and the saturating phase's precondition recorded as met. Verify the bundle's assertion list shows no `invalid` entries for the enforced assertions.
- [x] 8.3 Run `openspec validate performance-insight-and-smoke-gate --strict` and confirm the change's spec deltas match what was implemented (capabilities `performance-validation` added, `fargate-benchmark-harness` baseline capture modified).

## Workflow follow-up

- Archive the change after review; confirm the main specs gain `performance-validation` and the modified `fargate-benchmark-harness` requirement.
- Note in the archive notes that the `EffectiveNumCPU()` treatment of a sub-one-CPU quota is a separate change, made visible by `bench-host-check`.
