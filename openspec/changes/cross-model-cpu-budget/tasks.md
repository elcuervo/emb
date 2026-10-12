## 1. Process budget and compatibility

- [x] 1.1 Resolve fractional cgroup/affinity/scheduler allowance and validate `cpu_budget`, `cpu_reserve`, and positive finite model weights.
- [x] 1.2 Add process-wide FIFO admission around embedding, script, and image ONNX runs; release capacity on success and error.
- [x] 1.3 Apply the process plan before preload; preserve `LoadModel` behavior for existing callers and keep probe-failure single-row fallback.
- [x] 1.4 Add CPU allowance/layout fields to `EMB.INFO` and `EMB.STATS`, and document the current budget controls.

## 2. Out-of-the-box startup autotuning

- [x] 2.1 Add `calibrate` config with default on for automatic dimensions and an explicit off path; explicit workers and widths bypass tuning for their own dimensions.
- [x] 2.2 Build representative fixed short/typical/long inputs from the loaded tokenizer without recording user text; run warmup separately from repeated candidate trials.
- [ ] 2.3 Screen deduplicated thread-width/session-count candidates within the process CPU share and existing model-size memory estimate. Joint multi-model trials remain future benchmark work.
- [ ] 2.4 Score correct candidates by measured throughput, reject unavailable CPU accounting and p95 above twice the serial baseline, and prefer the lower-resource candidate on a tie.
- [ ] 2.5 On invalid fixtures, noisy measurements, deadline, unsupported paths, or unavailable metrics, keep the best validated candidate or select the documented one-worker/one-thread fallback and report why.
- [x] 2.6 Ensure probe failure is known before calibration so degraded models tune single-row inference and cannot retain batch-sized workers.

## 3. Runtime demand adaptation (deferred)

Deferred: the first release selects a safe layout at model load and enforces shared process admission. Online concurrency control needs a separate change with a meaningful sustained-load benchmark.

## 4. Local verification and documentation

- [ ] 4.1 Add model-free tests for candidate scoring, fake-clock dwell/hysteresis, budget fairness, CPU saturation, missing signals, rollback, multi-model skew, probe failure, memory/deadline fallback, and explicit overrides.
- [ ] 4.2 Add a seeded local benchmark mode for single-model, paired, serial/parallel, cold/warm-cache, and short/typical/long request mixes near the reported 10 pairs/s/task. Record offered/completed/dropped pairs, per-leg and pair latency, CPU seconds per pair/ORT row, RSS, errors, and selected layout.
- [ ] 4.3 Update config, commands, and benchmark documentation with defaults, precedence, startup cost, fallback behavior, and what the tuner can and cannot guarantee.
- [ ] 4.4 Run `go test ./...`, `just lint`, targeted race tests, `openspec validate cross-model-cpu-budget`, and a local two-model smoke test with no model tuning keys. Verify inferred settings, inference correctness, and `EMB.INFO`/`EMB.STATS` output.

Optional deployment follow-up: repeat the local benchmark on representative Linux ARM64 containers and compare with the production request/CPU baseline before changing rollout policy. This is useful evidence, not a prerequisite for implementing or locally completing this change.
