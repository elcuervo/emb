# Design

## Context

See `proposal.md` — Why. What shapes the approach:

- The autotuner's own signal is CPU-fraction based (`internal/registry/sampler.go` feeds `permit.Classify` with `(Δuser+Δsys)/(window × cores)`), and `cores` comes from `EffectiveNumCPU()`, which floors a cgroup quota to `max(1, quota/period)`. Measuring the loop on a host whose delivered CPU does not match what the server believes therefore tests nothing — this is the constraint the whole design hangs on.
- Existing measurement assets: `bench/fargate/` (builds a `linux/arm64` image, runs tiers under `docker run --cpus/--memory`, emits JSON with a `host` block, diffs with tolerances), `bench/script/{run,shape}.sh` (interleaved A/B with reply-identity failure, and the serial→burst→serial shape drive), `internal/server/script_bench_test.go` (in-process server helper, `EMB_BENCH_BUDGETS=1` gates, recorded `throughputScalingBaselineRatio`), `BENCHMARK.md` tables measured on an Apple M4.
- CI reality: `ci.yml` runs on `push`/`pull_request` to `main` only, with no `schedule` and no `workflow_dispatch`; the `changes` job classifies the diff into `code`/`sandbox`; `test-cgo` installs ONNX Runtime and libtokenizers, runs the CGo suites under a 1200 s wall-clock budget, reports its skip count, and then runs the coverage gate. No workflow sets `EMB_BENCH_BUDGETS`, so performance has never gated a pull request.
- The repository is public, so `ubuntu-24.04-arm` standard runners (4 vCPU / 16 GB, ARM64) are free and unlimited.

## Goals / Non-Goals

**Goals:**

- A regression gate on the autotune path that runs on every code change, is cheap enough to stay enabled, and fails loudly rather than quietly.
- A recorded reference run whose numbers are comparable over time: same host, same tolerance, explicitly invalid when its preconditions were not met.
- Make host validity a checked property, so an unqualified host can never silently become evidence.

**Non-Goals:**

- Replacing `EMB_BENCH_BUDGETS` (memory, latency parity, throughput scaling) or the existing golden baselines; re-recording them is not part of this change.
- Any change to server runtime behaviour, configuration keys, or `EMB.INFO`.
- Fixing `EffectiveNumCPU()`'s treatment of a sub-one-CPU quota (see the proposal's Non-goals); this design only makes it reportable.
- Absolute throughput targets, or comparisons between hosts and ISAs.

## Decisions

### D1 — Two tiers, one contract

A per-change smoke gate (in-process, in-repo fixture, ~1 minute, no download) asserts *behaviour*; a per-host reference run (real ARM64 Linux host, the deployed model set) records *numbers* and diffs them against that host's baseline. Both exercise the same contract, so a green smoke gate means the behaviour held, and a green reference run means it held at scale.

Alternatives: a single nightly reference run (a regression would sit for a day, and the gate would be edited out of the way when it flakes); extending `bench/fargate` per change (Docker plus a tier matrix: minutes to tens of minutes, and it measures throughput rather than the autotune contract).

### D2 — Nothing absolute gates anything

Throughput is gated two ways, both relative: against a fixed-concurrency control arm (`autotune: off` at the same cap) measured **in the same run**, and against a baseline recorded **on the same host** within the existing tolerances (median per-cell req/s ≥ −5%, p50 ≤ +10%). Behavioral assertions are exact equality or exact state, never tolerance.

Alternatives: absolute thresholds baked into the test (bound to one machine, and they rot — this is why the `BENCHMARK.md` tables cannot be a gate); a frozen JSON of expected numbers (same problem, plus it invites rubber-stamping when it needs updating).

### D3 — Real sampling windows, counter-based assertions

The smoke gate drives the real sampler and the real session pool with the production 1 s window, and asserts on state (`EMB.INFO` class, allowance, in-flight, run counters) rather than on wall-clock durations. Phase lengths follow from the controller's dwell (`autotuneShrinkAfter = 10`, `autotuneGrowAfter = 1`), so a shrink proof costs about ten windows and the whole gate stays under two minutes. The pure dwell/threshold arithmetic stays covered by the deterministic `internal/permit` unit tests; the smoke gate exists for the wiring those tests cannot reach (sampler → pool capacity → counters → `EMB.INFO`).

Alternatives: injecting a fake tick into the sampler (fast, but it removes exactly the integration this gate is for, and the repository already learned this lesson in `internal/pipeline/batcher_budget_test.go`, which replaced `time.Sleep` assertions with gated fakes); asserting on wall-clock phase durations (flakes under load, which is what CI is).

### D4 — Fixture choice: in-tree miniature for the gate, cached real model for the reference

The smoke gate uses `testdata/laya` (804 KB, committed, random weights, already shipped in the sandbox image) so it needs no network and no Docker; its preset is committed beside the fixture and **not** under `scripts/` or `website/repl/presets/`, so the site's stamped-digest check (`website/tools/stamp-presets.py`) is untouched. The reference tier uses the scripted model set (`models/gliner2`, ~380 MB) behind an `actions/cache` entry keyed by model, mirroring how `test-cgo` caches `models/minilm`.

Alternatives: `models/minilm` everywhere (the budget tier's job, and a download on every runner); a synthetic graph with no tokenizer (would not exercise the scripted path, which is where the autotuner lives).

### D5 — Saturation is asserted where it can be reached, and reported where it cannot

The saturating phase records the CPU fraction it actually achieved. When the gate runs on a host that cannot reach the threshold, the `saturated` assertion is reported as skipped, naming the host property — never as a pass — and the bundle marks the run invalid as a reference. The reference tier exists so the assertion is enforced somewhere every cycle, and `bench-host-check` is what tells the two situations apart before a run starts.

Alternatives: asserting unconditionally (flaky on shared runners, which ends with the assertion deleted); dropping the saturation assertion (leaves the one gate we know can be silently unfalsifiable); asserting only in the reference tier (loses the per-change signal entirely).

### D6 — Placement: gate in `internal/server`, tooling in `bench/`, CI in `test-cgo`

The gate is a Go test in `internal/server` next to the budget tests, sharing the in-process server helper, enabled by `EMB_BENCH_SMOKE=1` inside the existing `test-cgo` job — which already has ORT and libtokenizers, already reports its skip count, and has a 1200 s suite budget the gate must stay well inside (target: under 120 s, with the job's existing wall-clock check catching creep). Host-check, bundle and diff tooling live under `bench/`, which is outside the coverage surface (`scripts/coverage.sh gate`), so the gate itself does not disturb the per-package floors. The reference tier is a new `workflow_dispatch` job on `ubuntu-24.04-arm` — a new trigger for `ci.yml`, following the precedent in `release.yml`/`site.yml` — since nothing in `ci.yml` runs on a schedule today.

Alternatives: a separate `perf` job (a second ORT install, a second Go setup, and a new required check to keep green); a shell-only gate (loses the in-process server helper and the coverage-visible assertions); running the gate in `test` (no ORT there).

### D7 — The Fargate reference run drives itself from inside the task

Fargate's CPU quota is per *task*, not per container, and a client on another machine measures the network. So the reference run on Fargate is one container running the server, the driver and the benchmark client, with a `taskset` partition between them; on a host with enough vCPUs the same partition applies to `bench/script/*.sh` via the existing `app_cpus`/`bench_cpus` knobs.

Alternatives: a runner driving the task over the network (measures the wrong thing, and gives the client its own cores, which changes what the server's classifier sees); a sidecar container (shares the task quota, so the partition is meaningless).

### D8 — One bundle schema

The `host` block `bench/fargate/main.go` already emits becomes the fingerprint (architecture, effective CPU allowance and source, steal and throttle verdict) plus revision, model digests, configuration, per-cell metrics, and an assertion list with `pass`/`fail`/`invalid`/`skipped` outcomes. One diff path, one refusal rule: differing fingerprints never compare.

Alternatives: a second schema for the new runs (two diff paths, and no shared refusal rule); storing only in CI artifacts (nothing to compare against later, and no drift guard on the host itself).

### D9 — The regression catalogue is part of the deliverable

Each plausible failure is named with the assertion that catches it, so the gate's coverage is reviewable rather than implied:

| Planted or plausible regression | Assertion that catches it |
|---|---|
| Allowance dwell inverted / shrink made eager | class-and-allowance transition sequence (serial → burst → serial) |
| Saturation threshold raised out of reach | saturating phase reaches `saturated`, plus the reachability precondition |
| CPU gate fires while idle | serial phase must classify `latency` and log no recommendation |
| Reply-cache hits counted as traffic | cache-warm window is `idle` with frozen run counters |
| Allowance not restored after a burst | allowance returns to the cap on the first `throughput` window |
| Allowance exceeds the cap or dips below one | cap/floor assertions across all arms |
| Runtime reshape of sessions or threads | layout immutability across adaptation |
| Dispatch-wait accounting broken | a queued burst classifies `throughput` (wait/run ratio) rather than `latency` |
| Throughput collapse or extra copying on the scripted path | control-arm relative gate; the existing memory budget stays as the deep check |
| Reply drift under a profile or the kill switch | byte-identical replies across arms |
| Host silently unsuitable (quota below the presented CPUs) | `bench-host-check` verdict + bundle fingerprint rules |
| Precondition never reached but reported green | precondition outcome recorded per assertion; `invalid` is not `pass` |

## Risks / Trade-offs

- **A shared CI runner cannot reach saturation** → the assertion reports a skip naming the host property, the bundle marks the run invalid as a reference, and the reference tier (qualified host) enforces it; the smoke gate never silently passes it.
- **Timing flake in the gate** → relative gates, medians over repeated rounds, warmup excluded, counter-based assertions, and the client partitioned away from the server's cores; the pure math stays in deterministic unit tests. If the gate still flakes, the answer is a qualified host, not a looser tolerance.
- **Gate cost creeping inside the 1200 s `test-cgo` budget** → a stated ceiling (~120 s) with the job's existing wall-clock check as the backstop, and the gate runs on the in-repo fixture with no download.
- **Tolerances and baselines rot** → a baseline is re-recorded deliberately, its fingerprint and the fixture digests are in the bundle, and a fixture change that invalidates the baseline is visible as a digest change rather than a mysterious drift.
- **Duplication with the budget tier** → the smoke gate asserts behaviour (exact) and relative throughput; memory, latency parity and scaling stay with `EMB_BENCH_BUDGETS`, explicitly out of scope here.
- **The gate becomes a rubber stamp** → the self-verification requirement is part of the change: the gate must be shown red on each planted regression, and the skip count plus per-assertion outcomes keep a "green but nothing ran" run from looking like a pass.
- **A new CI trigger is a new failure surface** → the reference job is `workflow_dispatch` (not a required check), so it cannot block a merge while its host qualification is still being established.

## Migration Plan

1. Host check and bundle schema land first (no behaviour change, no CI change): `just bench-host-check`, the `bench/fargate` fingerprint and refusal rules.
2. Smoke gate lands with its self-verification, still not wired into CI; `just bench-smoke` proves it locally in `nix develop`.
3. `EMB_BENCH_SMOKE=1` is enabled in `test-cgo`, making it the first performance gate to run per change.
4. The ARM64 `workflow_dispatch` reference job and the docs land, with the first recorded baseline and its qualified host.

Rollback is per step and needs no runtime change: unset the env var, drop the job, keep the tooling. Nothing in steps 1–4 touches the server binary's behaviour, the RESP surface, or the configuration schema.

## Open Questions

- Which rung the free `ubuntu-24.04-arm` runner actually is: the first `bench-host-check` run there answers it. If it reports unqualified (steal above threshold), the reference tier moves to the rentable dedicated host or the Fargate task and the runner keeps only the smoke gate — the tasks and specs hold either way.
- Whether the reference run is `workflow_dispatch` only or also `schedule:`d — a one-line trigger change later, no artifact depends on the answer.
