# Tasks

## 1. Fix the capacity-profile precedence gap

- [x] 1.1 In `internal/registry/registry.go`, apply a profile's default spinning only when `cfg.AllowSpinning == nil`, and drop the `autotune = true` assignment from the `throughput` case so `cfg.AutotuneEnabled()` flows through. Verify with registry tests asserting: `capacity: latency` + `allow_spinning: false` creates sessions spinning off; `capacity: throughput` + `allow_spinning: true` keeps ORT spinning; `capacity: throughput` + `autotune: off` reports `script_autotune_active` 0 and a fixed allowance; and `capacity: latency` with nothing set still spins with the controller off.
- [x] 1.2 Run `go test ./internal/registry/ ./internal/config/ -count=1` and `just lint`; verify both pass and no existing profile/autotune test regresses.

## 2. Record the measurements in BENCHMARK.md

- [x] 2.1 Add an "Adaptive capacity" section to `BENCHMARK.md` carrying the out-of-the-box comparison (pre-change `c779fe5` vs candidate, `script_workers` unset), the per-second shape trace, and the controller/profile table, each with the reference host, corpus, and the exact reproduction command. Verify by re-reading the section against `design.md — Decision 3` and confirming every number matches a recorded run.
- [x] 2.2 Add the reproduction commands (`just bench-shape SHAPE=mixed PHASE=15s BURST=8 WORKERS=4` and the interleaved `just bench-script BASE=… CAND=…`) and the guardrail results (`OK: replies identical`, Ruby suite 203/203) to the same section. Verify each command is runnable as written from the repo root inside `nix develop`.

## 3. Configuration section (04)

- [x] 3.1 Add `capacity` and `autotune` rows to the model-options ledger in `website/docs/index.html`, matching the wording and defaults in `docs/configuration.md` and `internal/config/config.go`. Verify the rows parse as valid markup and every value matches the shipped defaults (`auto`/`auto`).
- [x] 3.2 Add a short profile statement naming the three creation-time layouts (`auto`, `latency`, `throughput`) beside the existing shared-sessions prose. Verify a reader can tell which layout each profile pins (spinning and initial callers) without leaving the section.
- [x] 3.3 State the precedence the fixed code now honors: an explicit `allow_spinning` and `autotune: off` win over a `capacity` profile. Verify by re-running the task 1.1 assertions and confirming each sentence matches an observed `EMB.INFO <model>` value.

## 4. Operations section (06)

- [x] 4.1 Add the autotune observable fields (`script_traffic_class`, `script_inflight`, `script_concurrency_current`, `script_concurrency_target`, `script_autotune_active`) to the observability ledger, with the `EMB.INFO <model>` form of the example. Verify the field names match `readAutotune` in `cmd/evalbench/main.go` and the `EMB.INFO` output of a running server.
- [x] 4.2 Add the CPU-saturation recommendation and the `autotune: off` kill switch, and how to read class against `inflight`. Verify against `internal/registry/sampler.go`'s recommendation string and `docs/operations.md#autotune-state`.

## 5. Benchmarks subsection and the three plates (07)

- [x] 5.1 Add the "Adaptive capacity" subsection to the benchmarks section with the out-of-the-box figures and the reproduction command, citing `BENCHMARK.md`. Verify no number appears that is not in the recorded `BENCHMARK.md` table.
- [x] 5.2 Draw plate 1 (the shape trace): class spans as mono caps labels over ruled spans, `inflight` as a faint ink area, `concurrency_current` as the single orange step line, the cap as a dashed hairline, phase boundaries labelled `serial c=1` / `burst c=8`. Verify it renders from the committed inline SVG over `file://` with no script, and that the caption carries the per-phase req/s, p50, and p99.
- [x] 5.3 Draw plate 2 (out of the box): a dumbbell per metric for pre-change vs candidate, with the two ×2.0 rows and the serial-parity row drawn. Verify the caption states each drawn value and that no value claims the ×2 for the controller alone.
- [x] 5.4 Draw plate 3 (the capacity profiles): a small-multiple scatter of serial p50 against burst req/s for `latency`, `auto`, and `throughput`. Verify the caption states the tradeoff values and the `auto` point is the accented one.
- [x] 5.5 Add the guardrail ledger beside the figures: identical replies under the A/B harness, the Ruby suite result, `EMB.READY` answering, and the saturation gate. Verify each statement is backed by the recorded `BENCHMARK.md` result.

## 6. Integration checks

- [x] 6.1 Run `impeccable detect --json` once over the changed files and compare finding-by-finding against the same run in a worktree of `HEAD`; verify the plates add no new finding.
- [x] 6.2 Load `website/docs/index.html` over `file://` at the 1086px frame and at 390px; verify no third-party request, no text below the 12px/14px floors, every plate scales, and every text colour meets WCAG AA against its ground.
- [x] 6.3 Re-read the adaptive-capacity subsection and confirm every claim is scoped per the `product-docs` delta: the out-of-the-box improvement is attributed to the adaptive-capacity line, and the controller's parity is stated.
- [x] 6.4 Run `go test ./... -count=1`, `openspec validate website-autotune-docs --strict`, and `just website-presets-check`; verify all pass and that no preset SHA changed.

## Workflow follow-up

- Archive this change after review; `product-docs` gains three requirements and `script-inference-performance` gains the precedence requirement.
