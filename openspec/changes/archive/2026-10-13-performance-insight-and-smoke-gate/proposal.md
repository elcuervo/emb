# Proposal

## Why

`emb` now ships *behavioural* performance claims, not only throughput tables: the runtime controller classifies each scripted model's traffic from process CPU utilization expressed as a fraction of the detected core budget (`(Δuser+Δsys) / (window × cores) ≥ 0.90`) and adapts the per-session concurrency allowance from that. Nothing in the repository lets a reader tell a real measurement from a flattering one, and nothing stops a regression in that loop from landing.

Three concrete gaps:

1. **Measurement hosts are unexamined, so a gate can be unfalsifiable.** Every number in `BENCHMARK.md` comes from one laptop (Apple M4) or the local Fargate emulation, and no artifact records what a host must be for the CPU-fraction gate to mean anything. A host that advertises *N* cores but delivers fewer — shared or burstable vCPU, steal from neighbours, a quota below the advertised count — makes `saturated` *unreachable*, turning the CPU-saturation safety gate into a claim no test can contradict. The deployed sandbox is that host: Fly `shared-cpu-*` enforces a 5 ms/80 ms quota per vCPU, which `parseCPUQuota` floors to `max(1, 0)` = 1, so on `shared-cpu-4x` the server plans for, and judges itself against, one CPU while the machine bursts to four.
2. **There is no cheap end-to-end test of the loop.** `internal/permit` has deterministic unit tests for the classifier and controller, but they never exercise the sampler, the session pool, or `EMB.INFO` together. The two existing integration layers are too expensive to run per change: `just bench-budgets` needs `models/minilm` downloaded plus minutes of wall clock, and `bench/fargate` needs Docker and a tier matrix. No workflow sets `EMB_BENCH_BUDGETS`, so performance has never gated a pull request at all.
3. **There is no *reference*.** A gate is only a reference if it is comparable over time: a recorded, host-qualified baseline with stated tolerances and a diff that refuses incomparable inputs. `bench/fargate` already writes a `host` block into its JSON, but nothing qualifies that host, nothing prevents diffing across two different hosts, and nothing distinguishes "ran and passed" from "ran and could not exercise the assertion".

## What Changes

- **A validation host contract, asserted instead of assumed.** `just bench-host-check` reports the effective CPU allowance (cgroup `cpu.max`, or the v1 pair, or scheduler/affinity) against `nproc`, the cgroup memory limit against host RAM, sampled `/proc/stat` steal and `cpu.stat` throttle deltas, the architecture, and the server's own view (`cpu_budget`, `intra_op_threads`, the `CPU calibration …` line, `EMB.INFO`). It returns a qualified/unqualified verdict and names the property that failed. It runs before any number is recorded.
- **Recorded evidence bundles.** Every recorded run emits one JSON bundle: host fingerprint, revision, model digests, configuration, per-cell metrics, and the outcome of every assertion. A bundle from an unqualified host is marked non-gold and cannot serve as a reference; a run whose precondition was never reached (the saturating phase did not reach the saturation threshold) is recorded as *invalid*, never as a pass; the diff tool refuses two bundles whose fingerprints differ. Comparability stops being a convention.
- **A reference performance test in two tiers, gated relatively.**
  - *Per change* — a ~1 minute smoke gate (`EMB_BENCH_SMOKE=1`) on the in-repo 804 KB `testdata/laya` miniature with no download, running inside the existing `test-cgo` job alongside its ORT and libtokenizers installs and its 1200 s suite budget. That job already reports its skip count so a green run cannot mean "the model-dependent tests did not run"; the gate inherits that honesty.
  - *Per host* — the same behavioural contract reproduced on a qualified ARM64 Linux host (`ubuntu-24.04-arm`, free for this public repository, or the Fargate ARM 4 vCPU/8 GB task that is the declared deployment target), recorded as a bundle and diffed against that host's recorded baseline within the existing tolerances (median req/s ≥ −5%, p50 ≤ +10%).
  - **No absolute threshold gates anything.** Throughput is compared against a fixed-concurrency control arm measured in the same run, and against a baseline recorded on the same host; a number captured on one host is never a gate on another. Percentile and throughput cells are medians of repeated rounds, arms are interleaved, and the load generator is pinned away from the server's cores where the host has enough of them (Fargate's quota is per *task*, so there the driver runs inside the task under a `taskset` partition).
- **The regression catalogue, wired to assertions.** Each plausible failure gets the test that catches it: an inverted or re-tuned allowance dwell; a saturation gate that never fires, or fires while idle; reply-cache hits counted as traffic; an allowance that does not return to the cap after a burst or exceeds it; a shrink that starves a burst; a runtime reshape of session or thread counts; dispatch-wait accounting that stops distinguishing queueing from compute; a throughput or memory collapse in the scripted path; and reply drift. Behavioral assertions are exact; only throughput is tolerance-gated.
- **Gate self-verification.** The gate ships with the check that it *fails* on planted regressions — an inverted dwell, a raised saturation threshold, cache hits counted as traffic, and a runtime layout change. A gate that cannot fail is not a gate.
- **Honest skips.** An assertion the current host cannot exercise (saturation on a shared runner) is reported with the failing host property and counted, never silently passed; the reference tier exists so the saturation assertion is enforced somewhere on every cycle.
- **Docs.** The host ladder with its costs and the reason each rung is or is not acceptable for evidence, the contract itself, the insight-versus-gate split, and the rule in `BENCHMARK.md` that absolute numbers are not comparable across hosts.

Not **BREAKING**: no server configuration key, wire protocol, reply format, embedding, or `EMB.INFO` field changes.

## Non-goals

- **Changing how `EffectiveNumCPU` treats a quota below one CPU.** The Fly `shared-cpu-*` collapse is a candidate defect; this change makes it visible, measurable and reportable and leaves the fix to its own change.
- **Replacing the budget tier.** `EMB_BENCH_BUDGETS=1` (memory, latency parity, scaling) keeps its role; the smoke gate is the cheap gate, not a substitute.
- **Changing existing tolerances, re-recording existing baselines, or adding configuration keys.**
- **Any absolute throughput target.** Cross-ISA and cross-host req/s comparisons stay out of scope.

## Capabilities

### New Capabilities

- `performance-validation`: the validation host contract and its preflight, the recorded evidence bundle and its comparability rules, the two-tier reference performance test (per-change smoke gate and per-host reference run), and the gate's self-verification.

### Modified Capabilities

- `fargate-benchmark-harness`: baseline capture SHALL require a host that passes the validation host contract and SHALL record that host's fingerprint beside the captured cells, so a baseline is only ever compared with a run from the same host.

## Impact

- **New:** host-check and evidence-bundle tooling under `bench/` (outside the coverage surface), the smoke gate test in `internal/server/` (next to the budget tests and their in-process server helper), `just bench-smoke` / `just bench-host-check` targets, a `docs/` page for the host ladder and contract.
- **Modified:** `bench/fargate/main.go` (contract-qualified capture, shared bundle schema, fingerprint in the diff), `bench/script/{run,shape}.sh` (preflight + bundle), `justfile` (pass the ORT library path on Linux as well as macOS), `.github/workflows/ci.yml` (`EMB_BENCH_SMOKE` in `test-cgo`; a new `workflow_dispatch` ARM64 reference job, since ci.yml has no scheduled or dispatched trigger today), `BENCHMARK.md`, `docs/testing.md`.
- **Cost:** the arm64 runner is free for this public repository; a Hetzner ARM VM (`CAX21`, 4 vCPU/8 GB) is $0.020/h and `CCX13` (2 dedicated vCPU) $0.081/h when a dedicated thread is needed; Fargate ARM 4 vCPU/8 GB is $0.158/h, about $0.05/h on Fargate Spot. Fly `shared-cpu-*` is excluded as a reference host by the contract.
- **Unchanged:** server behaviour, the RESP surface, `EMB.INFO` fields, the Dockerfile, and the existing budget gates and golden baselines.

## Validation

The change proves itself with the artifacts it adds, and it is only done when the gate has been shown to fail:

```
$ nix develop
$ just bench-host-check          # qualified / unqualified, with the failing property named
$ just bench-smoke               # gate: behavioural + relative, ~70 s, no download
$ bash bench/smoke/verify-gate.sh  # the gate must FAIL on each planted regression
$ just bench-shape && just bench-script BASE=./bin/emb CAND=./bin/emb   # the reference host's shape and A/B runs
$ just bench-fargate-diff <before> <after>                  # same-host diff, cross-host refused
```

The dispatched reference job (`reference-arm64`) runs the same three steps on
`ubuntu-24.04-arm` and uploads the host contract's JSON report beside the shape
and A/B logs as its record.

- Each planted regression must turn the gate red and name the assertion it broke; the unmodified tree must pass.
- `just bench-host-check` must report the Fly `shared-cpu-*` quota collapse and the Apple M4 laptop as unqualified, and a contract-passing ARM64 host as qualified.
- `EMB_BENCH_BUDGETS=1` and the existing golden baselines keep working unchanged.
