# Tasks

## 1. Incident evidence and known-bad reproduction

- [ ] 1.1 Collect the deployed known-good and pre4 binary/image digests, Go/ORT versions, model/tokenizer/config digests, cgroup membership + mountinfo + quota/period + affinity, memory limit, startup CPU-plan/calibration logs, request/health/recovery deadlines and restart/OOM events into a redacted `bench/regression/incident-profile.json`. Verify every field is filled from a source or explicitly marked `unavailable` with the reason; no invented values.
- [ ] 1.2 Confirm `v0.4.3.pre3` (`a0ffc5b`) matches the deployed known-good artifact, or record the actual known-good revision in the profile. Verify by digest comparison.
- [ ] 1.3 Add `bench/regression/` profiles for the public-model control pair (minilm+bge, `test-two-models.yaml`) with finite `max_concurrent_requests`, `max_connections` and preload flags, plus the incident profile once 1.1 lands. Verify each profile starts `bin/emb` through `cmd/emb` and reports `cpu_budget_enabled 1`.

## 2. Fail-closed workload driver (reuse existing inference pair driver)

- [x] 2.1 Move the open-loop scheduler/RESP measurement from `internal/server/inference_bench_test.go` into the `bench/fargate` harness so it drives an external server address; keep the benchmark as a thin wrapper. Verify `go test ./bench/fargate/...` and the existing `BenchmarkInferencePairs` still build.
- [x] 2.2 Decode replies with the existing `internal/resp` decoder: count a pair successful only for valid float32 embeddings of the expected dimension with finite values; classify RESP errors, nulls, malformed replies and transport failures as errors. Verify with a unit test feeding a `-ERR` reply, a null and a short blob.
- [x] 2.3 Add per-request connection/read deadlines and a bounded final drain; classify every scheduled pair exactly once as success/error/timeout/dropped and assert `offered == sum`. Measure throughput from wall elapsed time and latency from both scheduled time and wire-start time. Verify with a fake server that stalls one reply (timeout counted, drain bounded).
- [x] 2.4 Prewarm the warm corpus before measuring, keep cold inputs unique, cycle short/typical/long per request, and report actual inference counters (from `EMB.INFO` deltas) instead of requested legs. Verify warm phase shows zero cache misses and cold phase all misses in a local run.
- [x] 2.5 Fix `bench/fargate` mixed-workload req/s to use wall elapsed time, literal p90, and explicit `__rand_int__` randomization for unique text. Verify with existing `bench/fargate/main_test.go` plus a case for each fix.
- [x] 2.6 Document the driver in `docs/performance-validation.md` and `bench/fargate/README.md`. Verify documented commands run as written.

## 3. Evidence bundle and comparator

- [x] 3.1 Extend the versioned Fargate bundle with artifact digests, model/tokenizer/config/corpus hashes, CPU model and runtime versions, server-observed limits (from each arm's `EMB.STATS`), phase counts, correctness, latency quantiles, admission/inference timing, CPU/throttle/RSS, startup/calibration metadata, recovery and named assertions; mark missing older-release fields `unavailable`. Verify bundle round-trip test.
- [x] 3.2 Make the comparator fail closed: reject empty/duplicate/missing cells, mismatched input digests, invalid required assertions, zero-success controls, non-approved baselines and incomplete runs before tolerances; iterate the required matrix, not baseline keys. Verify with tests for each rejection and a passing good-vs-good bundle.
- [x] 3.3 Implement the verdict: ≥3 interleaved rounds, good-vs-good noise check, candidate ≥95% known-good throughput, pair p50/p95/p99 ≤110%, ≥1000 successful pairs and ≥120 s per gated tail cell, zero unexpected errors/timeouts/drops at the sustainable rate; noisy evidence is `inconclusive`. Verify with synthetic bundles for pass, fail, collapse and inconclusive.
- [x] 3.4 Add self-verification cases (application-error replies, empty bundle, zero control work, deliberate throughput collapse) that must fail their named assertions, and a good-vs-good comparison that must pass. Verify via `go test ./bench/fargate/...`.

## 4. Unified resource resolution

- [x] 4.1 Resolve cgroup v1/v2 controller mounts and roots from `/proc/self/mountinfo` + membership in `internal/registry/cgroup.go`; walk visible ancestors and take the tightest quota; carry provenance for missing/unreadable data. Verify with synthetic fixtures: combined v1 mount, hybrid, remapped root, unlimited child under limited parent, malformed files, affinity tighter than quota.
- [x] 4.2 Route `EffectiveNumCPU`, `autoTuneWorkers`, legacy defaults and `bench/hostcheck` through the same resolution, removing the duplicate root-only parser while preserving `cpu_budget: off` semantics and reserve tiers. Verify with `xd://lsp` references showing no remaining direct callers of the old parser and existing `hostcheck_contract_test` passing.
- [x] 4.3 Normalize sampler CPU by fractional bandwidth and actual monotonic elapsed time; treat unavailable accounting as no headroom. Verify `internal/permit` / sampler tests for a 0.5-CPU saturated window, a delayed 2 s sample, and missing accounting.
- [x] 4.4 Update `docs/configuration.md` and `docs/performance-validation.md` (sub-one-CPU treatment). Verify docs match the new log line fields.

## 5. Borrowable capacity and memory admission

- [x] 5.1 Change calibration's candidate ceiling from the model's static share to the process budget intersected with explicit dimensions and memory headroom; keep weights for scheduling only. Verify a registry test where an idle configured peer no longer limits the active model's candidates, and a runtime repeat of the two-model probe (`GOMAXPROCS=4`, minilm busy, bge idle) shows width > 1 or peak tokens > 1.
- [x] 5.2 Add process-wide memory reservation (current RSS, remaining configured model reservations, cache allowance, external ONNX data) serialized across concurrent loads; fall back to a smaller valid layout with a memory-limited reason. Verify tests for two concurrent lazy loads and a larger candidate that cannot fit.
- [x] 5.3 Replace the FIFO `permit.Budget` with per-model FIFO queues and weighted work-conserving turns, model identity in the acquire API, and a bounded bypass rule for wide heads; migrate embedding, script, image and calibration wrappers together. Verify with gated-fake tests (no sleeps) for fairness, idle borrowing, wide-request progress and error release, plus `go test -race ./internal/permit/ ./internal/registry/`.
- [x] 5.4 Replace the script `ceil(width/callers)` charge with active-session plus concurrent-caller accounting tied to the permit pool lease. Verify tests for one caller, full caller cap, multiple sessions and mixed paths.
- [x] 5.5 Update `docs/configuration.md` (weights, layouts, memory limits, `calibrate: false` semantics). Verify text matches behavior in 5.1–5.4.

## 6. Calibration cost and readiness

- [x] 6.1 Measure candidate total cost from before session creation through close and use it for deadline prediction; keep scoring on inference timing and store immutable outcome metadata (source, reason, duration, ceiling). Verify with fake-clock calibration tests where construction dominates.
- [x] 6.2 Make `calibrate: false` select a documented process-capable, memory-safe layout distinct from `cpu_budget: off`. Verify registry test.
- [ ] 6.3 Ensure configured embedding/script/image preloads, including calibration, finish before `SetReady`; failures abort startup naming the model; lazy paths stay lazy and observable. Verify a server test where `EMB.READY` is not OK during preload calibration, and that `EMB.STATS`/`EMB.INFO` never initialize lazy paths.
- [ ] 6.4 Document readiness semantics (registration vs warm readiness) in `docs/operations.md`. Verify wording against 6.3 behavior.

## 7. Bounded cancellable admission

- [ ] 7.1 Add `cpu_admission_timeout` (existing duration parser; zero/omitted unlimited; negative invalid) to `internal/config`. Verify config validation tests.
- [ ] 7.2 Change admission to `(release, error)` with cancellation and timeout; cancelled/expired waiters leave the queue atomically and wake successors; grant/cancel races have one owner. Return `ERR busy: CPU admission timeout` through existing per-command error paths. Verify deterministic tests for expiry, grant/cancel race, error release and shutdown, with `-race`.
- [ ] 7.3 Wire cancellation to registry shutdown and available request boundaries without destroying resources of a running native call. Verify a shutdown test with one queued and one running call.
- [ ] 7.4 Document the timeout and the recommended finite production profile (request cap, connection cap, timeout below application deadline) in `docs/configuration.md` and `docs/operations.md`. Verify the profile in 1.3 uses it.

## 8. Observability

- [ ] 8.1 Add `cpu_admission_wait_us`, `cpu_admission_waiters`, `cpu_admission_timeouts` to `EMB.STATS` and per-path admission wait; exclude global waiting from `run_us`. Verify server tests for counts, RESP array parity, and that a gate-only wait does not increase `run_us`.
- [ ] 8.2 Add calibration source/reason/duration/ceiling and per-path initialization state to `EMB.INFO`; update `internal/embtop` parsing and telemetry mappings. Verify embtop and telemetry tests and that rendering does no cgroup I/O or lazy init.
- [ ] 8.3 Update `docs/commands.md` for new fields and corrected timing semantics. Verify field list matches server output.

## 9. Production-policy smoke and CI gates

- [ ] 9.1 Add a production-entrypoint smoke (real `NewWithConfig` path, two models, automatic budget) that fails on a bypassed governor, zero successful inference or any RESP error; add nonzero-work/error assertions to the existing script smoke throughput arms. Verify it runs under `EMB_BENCH_SMOKE=1` in `test-cgo`.
- [ ] 9.2 Add an ARM regression job (relevant-path triggered) running the control profile, and make `.github/workflows/release.yml` publication depend on the incident profile plus known-bad discrimination; missing fixtures or runners block. Verify with `act` or a dispatched run that a skipped/invalid job blocks publication and artifacts are retained.
- [ ] 9.3 Update `docs/testing.md` and `justfile` targets for the new gates. Verify `just --list` and documented commands.

## 10. Integration verification

- [ ] 10.1 Run `just test`, `just lint`, `just deadcode`, `just cover`, `go test -race ./internal/permit/ ./internal/registry/ ./internal/server/`, `just bench-smoke` and `bash bench/smoke/verify-gate.sh` inside `nix develop`. Verify all pass.
- [ ] 10.2 Run the three-arm comparison (known-good, pre4, candidate) on the incident profile on a linux/arm64 host with identical limits. Verify known-good passes, pre4 fails the intended service assertion (not a startup/fixture failure), candidate passes unchanged thresholds; retain the bundle.
- [ ] 10.3 Canary the candidate with required paths preloaded and finite request/connection/admission limits; record throughput, tails, admission pressure, CPU and RSS against the known-good envelope. Verify no OOM/restart and recovery within the declared deadline.
- [ ] 10.4 Run `openspec validate fix-pre4-inference-capacity --strict`. Verify it passes.

## Workflow follow-up

- If 10.2 does not reproduce the pre4 failure, report the discrepancy and revise this change before claiming a fix.
- Archive after review; coordinate shared-file changes with the active `performance-insight-and-smoke-gate` change (its task 8.2 stays its own).
