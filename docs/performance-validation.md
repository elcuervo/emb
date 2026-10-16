# Performance validation

`BENCHMARK.md` reports measured numbers. This page defines which hosts those
numbers may come from, what a recorded run is, and how a regression in the
autotune path fails a pull request. Nothing here changes server behaviour: it
is the contract the measurement tooling enforces.

## Why the host has to be checked

The runtime controller classifies each scripted model's traffic from process CPU
utilization expressed as a fraction of the CPU budget the server believes it
has:

```
(Δuser + Δsys) / (window × cores) ≥ 0.90   →  saturated
```

`cores` is `registry.EffectiveNumCPU()`, which floors a cgroup quota to
`max(1, quota/period)`. A host that presents *N* cores but delivers fewer —
shared or burstable vCPU, a quota below the presented count, steal from
neighbours — makes `saturated` unreachable, and turns the CPU-saturation safety
gate into a claim no test can contradict. Fly `shared-cpu-*` is the worked
example: it enforces a 5 ms/80 ms quota per vCPU, which floors to a budget of
one CPU, so the server plans for and judges itself against one CPU while the
machine bursts to four.

## The host contract

```bash
just bench-host-check            # qualified / unqualified, with the failed property
just bench-host-check -json      # the recordable form
```

It prints the architecture, the effective CPU allowance and the source of that
allowance, the memory limit against host memory, the server's own detected
budget, and sampled CPU steal and throttle deltas over a one-second window. It
exits non-zero when the host is unqualified.

| Property | Qualified when | Why it matters |
|---|---|---|
| platform | `linux/arm64` | the deployment target; a number captured on another ISA is not comparable with it |
| cpu quota | no quota enforced, or the quota is within 10% of the presented CPUs | a quota below the presented CPUs makes the server plan from the smaller number while the host presents more |
| cpu steal | measurable, and under 1% | CPU the hypervisor takes away is CPU the classifier cannot see |
| cpu throttle | report-only (a ratio above 10% is named) | a burstable plan already throttling explains lost throughput |

Every failed property is named, not just the first. The probe reuses the
server's own resolution (`registry.EffectiveCPUAllowance`) rather than a second
cgroup parser, so the report and the server cannot disagree about the budget the
server plans from. When the host exposes no cgroup CPU file the report says the
allowance comes from the scheduler and affinity limits, and reports no quota.

Recorded on the development machine (Apple M4, 10 CPUs) when this page was
written:

```text
host: darwin/arm64, 10 presented CPUs
cpu: 10.00 CPUs from scheduler/affinity (no cgroup quota), scheduler 10, affinity 10, server budget 10
memory: host limit 25769803776 bytes of 25769803776 host bytes
steal: unmeasured
throttle: unmeasured
verdict: unqualified: platform darwin/arm64 is not the deployment target linux/arm64; cpu steal is not measurable on darwin/arm64
```

Two properties fail there, and both are honest: it is not the deployment
platform, and neither steal nor throttling can be measured on it.

**A run on an unqualified host is not evidence.** It may be used to explore and
to debug, never to record a baseline: `bench/fargate -mode baseline` refuses an
unqualified host and names the failed property.

## The host ladder

| Rung | Host | Cost | Use |
|---|---|---|---|
| free CI | `ubuntu-24.04-arm` (4 vCPU / 16 GB, GitHub standard runner) | free for this public repository | the per-change reference run, if it reports qualified |
| cheap VM | Hetzner `CAX21` (4 vCPU / 8 GB, ARM) | $0.020/h | reference runs that do not need a dedicated core |
| dedicated | Hetzner `CCX13` (2 dedicated vCPU) | $0.081/h | reference runs that do, where steal makes the runner unqualified |
| declared target | Fargate ARM 4 vCPU / 8 GB | $0.158/h ($0.05/h on Fargate Spot) | the deployment shape; the driver runs inside the task under a `taskset` partition, because the quota is per task |
| excluded | Fly `shared-cpu-*` | — | unqualified by the quota property (the 5 ms/80 ms quota floors to a one-CPU budget) |
| excluded | laptops (Apple M4) | — | not the deployment platform and steal is unmeasurable: fine for exploration, never for a baseline |

The free `ubuntu-24.04-arm` runner is the first rung to try. If it reports
unqualified — steal above the threshold is the likely reason on a shared
runner — the reference tier moves down the ladder to a rentable host with
dedicated vCPU, and the runner keeps the smoke gate.

## Evidence bundles

Every recorded `bench/fargate` run emits one JSON bundle: `revision`, `created`,
`host` (the fingerprint), `model`, `config`, `cells`, and `assertions`.

- `host` is the fingerprint: `os`, `arch`, `presented_cpus`,
  `allowance{cpus,source,quota,scheduler,affinity}`, `memory_limit_bytes`,
  `server_budget`, `steal_verdict`, `throttle_verdict`, plus the digest
  `identity`, the contract verdict `gold`, and `failed_property` /
  `failed_properties` when the host is unqualified.
- `model` names the files actually measured with their `path`, `bytes` and
  `sha256` (`onnx`, `tokenizer`), so a model change shows up as a digest change
  rather than as drift.
- `config` records `platform`, `model`, `max_length`, the tier/client/pipeline
  matrices, `count_per_cell` and the diff tolerances (`req_gate`, `p50_gate`).
- `assertions` records `{name, outcome, detail}` with outcome
  `pass`/`fail`/`invalid`/`skipped`. `host-contract` passes only on a qualified
  host and otherwise fails naming the property; `cells-complete` passes only
  when every requested cell was measured, and is `invalid` when the run could
  not finish the matrix. `invalid` is an unmet precondition and `skipped` is an
  assertion the host could not exercise: neither is ever a pass.

### The fingerprint identity rule

`host.identity` is a digest of the stable host shape (platform, presented CPUs,
effective allowance and its source, scheduler and affinity limits, memory limit,
server budget). The sampled `steal_verdict` and `throttle_verdict` are recorded
beside it but deliberately excluded: they follow the load on the host rather
than identify it, and a key that drifted with them could never be compared with
itself. Two bundles with the same `identity` are comparable even when their
sampled verdicts differ.

### Refusal rules

- **Baseline capture** (`just bench-fargate-baseline`) refuses an unqualified
  host: it prints the verdict and the failed property, exits non-zero, and
  writes nothing. Only a run that passes the host contract may become a
  reference.
- **Diff** (`just bench-fargate-diff`) refuses two bundles whose
  `host.identity` differs, printing both fingerprints field by field, and exits
  non-zero.
- **Pre-fingerprint bundles are refused too.** The committed
  `bench/fargate/baseline.<sha>.json` files predate the fingerprint, so a diff
  involving them is refused (they carry no identity) rather than silently
  compared; re-record them on a qualified host.
- A `run` bundle from an unqualified host is still written, marked
  `"gold": false`, and is a usable candidate as long as its fingerprint identity
  matches its baseline's.

### Running the recorded tier

```bash
just bench-host-check                                      # the contract, with the failed property
just bench-fargate-baseline                                # qualified host only → bench/fargate/baseline.<sha>.json
just bench-fargate                                         # any host → bench/fargate/out/run.<sha>.<ts>.json
just bench-fargate-diff <baseline.json> <candidate.json>    # same-host, per-cell PASS/FAIL
```

The diff gates inference-bound cells (<5000 req/s) at req/s ≥ −5% and p50 ≤ +10%,
and saturated cells at p50 only.

## The per-change smoke gate

```bash
just bench-smoke          # EMB_BENCH_SMOKE=1, ~70 s, no model download, no Docker
```

The gate drives the production sampler (its real one-second window) and the real
session pool against the committed `testdata/laya` miniature and the fixture
preset beside it (`testdata/laya/smoke.lua` — fixed inputs, no ARGV), and
asserts on state rather than on wall-clock durations. Phase lengths follow from
the controller's own dwell, which is why a shrink proof costs ten sustained
latency windows.

What it asserts, and the regression each assertion catches:

| Assertion | Regression it catches |
|---|---|
| `class-sequence` | a traffic shape that stops classifying: latency → idle → latency → throughput/saturated → latency |
| `allowance-halved` | an inverted or eager shrink dwell (the allowance halves only after the sustained quiet window) |
| `allowance-restored` | an allowance that does not return to the cap on the first throughput window |
| `allowance-bounds`, `allowance-damped` | an allowance above the cap, below one, or changed on every window |
| `layout-immutable` | a runtime reshape of the session or thread layout while the allowance adapts |
| `saturation`, `saturation-enforced` | a raised saturation threshold: the burst reaches the 0.90 CPU fraction but the class never fires |
| `unreachable-reported-skipped` | an unreachable phase reported as a pass instead of a skip |
| `kill-switch` | `autotune: off` no longer holding concurrency at the configured value |
| `profile-latency`, `profile-throughput` | a capacity profile that stops pinning its layout or misreports its autotune state |
| `cache-idle` | reply-cache hits counted as traffic (a cache-served window must stay idle with frozen run counters) |
| `reply-identity` | reply drift under a profile or the switch |
| `control-arm` | a throughput collapse: the adaptive arm is gated against a fixed-concurrency control arm measured in the same run, medians over three rounds with the warmup excluded, at a 15% floor |
| `partition-reported` | the gate no longer reporting the CPU partition it ran under |

The gate prints every outcome and one summary line per arm, each naming the arm
it summarizes:

```text
emb-smoke: arm=autotune assertions=17 pass=16 fail=0 skipped=1
emb-smoke: arm=control assertions=12 pass=12 fail=0 skipped=0
```

`lint`, `test`, and the CGo CI job fail when an arm's own line is missing, so a
green run cannot mean the gate skipped. An assertion the host cannot exercise is
reported as `skipped` with the host property that prevented it — the saturating
burst above is skipped on a host whose CPU fraction cannot reach 0.90 — and the
one-core arm (`saturation-enforced`) is where that assertion is enforced per
change instead. Verified on the development machine: 16 pass, 1 skip, 0 fail,
67 s.

## Regression gate for the plain embedding path

`just bench-plain-emb` (also selected by `-run Smoke`, so `just bench-smoke` and
the sweep run both gates) drives the plain `EMB` path in-process against
`testdata/plain-emb` — an in-repo miniature, so no model download — through
serial → burst → recovery and six arms (twelve assertions):

| Arm | What it drives | Assertions |
|---|---|---|
| `plain` | bulk `EMB`, batching on, an explicit `cpu_budget`, reply cache, preloaded | `plain-emb-pinned`, `plain-emb-latency`, `admission-wait-reported`, `drain-after-disconnect`, `health-answers`, `cache-idle` |
| `pool` | batching off, several sessions at one thread against a small budget | `layout-concurrency`, `queue-bounded`, `admission-bounded` |
| `deadline` | the same shape with the admission bound derived from the measured serial request | `admission-bounded` (expiry, not only latency) |
| `layout` | the policy layout with calibration off | `calibration-off-request-path` |
| `lazy` | no `preload`: the model is not ready at boot and the first request drives the off-request-path load | `calibration-off-request-path` (the load must not be on the request) |
| `fair` | two models over one budget: a wide run holds the budget while a narrow probe arrives | `admission-fair` |

Bounds are derived per host from a measured serial request rather than pinned to
a constant (latency, admission and fairness each use their own multiple), so a
faster or slower runner scales instead of flaking; a phase that cannot be
exercised is reported as `skipped` with the host property, never as a pass.
`plain-emb-pinned` compares digests of the returned embeddings and says which
text took the looser per-component tolerance when a digest differs (the fixture
README records how the pins are regenerated).

## Gate self-verification

```bash
bash bench/smoke/verify-gate.sh     # ~6 min: the clean tree plus one gate run per mutant
```

A gate that cannot fail is not a gate. The sweep applies each committed
regression under `bench/smoke/mutants/` to a scratch tree — a detached worktree
at `HEAD` plus the current tree's changes, so the working tree is never touched
— and requires the gate to go red **and name the assertion that caught it**.
The catalogue is one patch per named regression, with the assertion each one
must trip:

| Mutant (`bench/smoke/mutants/`) | Planted regression | Assertion that must catch it |
|---|---|---|
| `01-eager-shrink.patch` | the allowance dwell inverted: shrink made eager, so the allowance halves on the first latency window | `allowance-halved` |
| `02-saturation-out-of-reach.patch` | the saturation threshold raised out of reach, so the class can never fire | `saturation-enforced` |
| `03-cache-hits-count-as-traffic.patch` | a reply-cache hit counted as a run, so a cache-served window stops being idle | `cache-idle` |
| `04-layout-reshape.patch` | the reported session count follows the live allowance: a runtime reshape of the layout | `layout-immutable` |
| `05-fifo-head.patch` | admission considers only the head of the queue: no borrowing past a blocked wide waiter | `admission-fair` |
| `06-admission-unbounded.patch` | the configured admission bound is ignored, so a command may wait forever | `admission-bounded` |
| `07-queue-unbounded.patch` | the admission queue is never refused | `queue-bounded` |
| `08-single-session-layout.patch` | one session holding the whole share, so a run fills the budget | `layout-concurrency` |
| `09-calibration-on-request.patch` | the request path loads (and calibrates) a model synchronously | `calibration-off-request-path` (the `lazy` arm: a preloaded arm would never reach the load) |

Mutants `05`–`09` are the plain-`EMB` tier-2 regressions: the scripted gate
cannot see an admission stall, so they are caught by `TestPlainEMBSmoke`, which
`-run Smoke` also selects.

Recorded sweep on the development machine:

```text
== clean tree (must pass) ==
✓ clean tree: gate green (emb-smoke: arm=autotune assertions=17 pass=16 fail=0 skipped=1 emb-smoke: arm=control assertions=12 pass=12 fail=0 skipped=0)

== 01-eager-shrink.patch (must fail on: allowance-halved) ==
✓ 01-eager-shrink.patch: red, and named allowance-halved (emb-smoke: arm=autotune assertions=17 pass=15 fail=1 skipped=1 emb-smoke: arm=control assertions=12 pass=12 fail=0 skipped=0)

== 02-saturation-out-of-reach.patch (must fail on: saturation-enforced) ==
✓ 02-saturation-out-of-reach.patch: red, and named saturation-enforced (emb-smoke: arm=autotune assertions=17 pass=15 fail=1 skipped=1 emb-smoke: arm=control assertions=12 pass=12 fail=0 skipped=0)

== 03-cache-hits-count-as-traffic.patch (must fail on: cache-idle) ==
✓ 03-cache-hits-count-as-traffic.patch: red, and named cache-idle (emb-smoke: arm=autotune assertions=17 pass=15 fail=1 skipped=1 emb-smoke: arm=control assertions=12 pass=12 fail=0 skipped=0)

== 04-layout-reshape.patch (must fail on: layout-immutable) ==
✓ 04-layout-reshape.patch: red, and named layout-immutable (emb-smoke: arm=autotune assertions=17 pass=15 fail=1 skipped=1 emb-smoke: arm=control assertions=12 pass=12 fail=0 skipped=0)

== 05-fifo-head.patch (must fail on: admission-fair) ==
✓ 05-fifo-head.patch: red, and named admission-fair (emb-smoke: arm=autotune assertions=17 pass=16 fail=0 skipped=1 emb-smoke: arm=control assertions=12 pass=11 fail=1 skipped=0)

== 06-admission-unbounded.patch (must fail on: admission-bounded) ==
✓ 06-admission-unbounded.patch: red, and named admission-bounded (emb-smoke: arm=autotune assertions=17 pass=16 fail=0 skipped=1 emb-smoke: arm=control assertions=12 pass=11 fail=1 skipped=0)

== 07-queue-unbounded.patch (must fail on: queue-bounded) ==
✓ 07-queue-unbounded.patch: red, and named queue-bounded (emb-smoke: arm=autotune assertions=17 pass=16 fail=0 skipped=1 emb-smoke: arm=control assertions=12 pass=11 fail=1 skipped=0)

== 08-single-session-layout.patch (must fail on: layout-concurrency) ==
✓ 08-single-session-layout.patch: red, and named layout-concurrency (emb-smoke: arm=autotune assertions=17 pass=16 fail=0 skipped=1 emb-smoke: arm=control assertions=12 pass=11 fail=1 skipped=0)

== 09-calibration-on-request.patch (must fail on: calibration-off-request-path) ==
✓ 09-calibration-on-request.patch: red, and named calibration-off-request-path (emb-smoke: arm=autotune assertions=17 pass=16 fail=0 skipped=1 emb-smoke: arm=control assertions=12 pass=11 fail=1 skipped=0)

verify-gate: every mutant was caught and the clean tree passed
```

The `assertions=17` in each line is the *scripted* gate's summary: `-run Smoke`
runs both gates, each writing its own `emb-smoke: arm=<arm> …` line, and
`gate_summary` prints every arm's line (de-duplicated, sorted) rather than the
last match. The plain gate's own count (twelve) is in the same log; only the
exit status decides the sweep.

Two things the sweep depends on that are easy to get wrong, both fixed here:

- **The working tree's changes must survive the loop.** `verify-gate.sh` applies
  each mutant to a tree that already carries the uncommitted working diff;
  restoring with `git checkout -- .` between mutants reverts that diff too, so
  the mutants are tested against `HEAD`. A mutant only the working tree's new
  coverage can catch — `09-calibration-on-request`, reached by the `lazy` arm —
  then falsely passes. The loop re-applies the diff after the restore.
- **A mutant patch's context is the code it edits.** Mutants are commit-ready
  unified diffs; editing the lines around one (e.g. a comment above
  `plan.QueueBound`) invalidates its context and the patch stops applying. Keep
  `07-queue-unbounded.patch` in step with `registry.go` when that region moves.

The script exits non-zero when a mutant passes, when a mutant fails without
naming its assertion, or when the clean tree is red, so a green sweep is the
statement that the gate has teeth.

## The reference tier (dispatched by hand)

The per-host reference run is a hand-dispatched job in `ci.yml`
(`reference-arm64`, `runs-on: ubuntu-24.04-arm`). It is not a required check: it
never runs on a pull request, so it cannot block a merge while the host ladder's
first rung is still being established.

A dispatch runs, in order:

1. `bench-host-check` — its JSON report is the run's fingerprint record, and the
   step fails on an unqualified host, because a run on an unqualified host is
   not evidence.
2. `bench/script/shape.sh` — the serial → burst → mixed shape drive, printing
   the classified traffic class and the concurrency allowance per phase.
3. `bench/script/run.sh` with the same binary as both arms — the interleaved A/B
   harness against itself, which is how this host's noise floor is established
   before any baseline is recorded on it.
4. `bench/fargate -mode run` — the evidence bundle, diffed against itself: the
   fingerprint, the revision and model digests, the per-cell metrics, and every
   assertion outcome. The tiers are the Fargate ones this host presents, and the
   step fails unless the bundle is a valid reference (`gold`, with nothing
   `fail` or `invalid` among its assertions).
5. `bench-out/reference/` is uploaded as an artifact even when the contract
   refuses the host, so the verdict is always inspectable.

The scripted model set (`models/gliner2`) and the minilm fixture
(`models/minilm`) are cached with `actions/cache`, the same way `test-cgo`
caches `models/minilm`.

### The first qualified run

Dispatched `reference-arm64` on the `perf-validation-smoke-gate` branch (run
`38315084333`, 5m46s, every step green; the other jobs skipped). It answers the
ladder's open question: the free runner is rung 1, and it qualifies.

```json
{
  "os": "linux",
  "arch": "arm64",
  "presented_cpus": 4,
  "allowance": { "cpus": 4, "source": "scheduler/affinity", "quota": false, "scheduler": 4, "affinity": 4 },
  "memory": { "limit_bytes": 16722010112, "host_bytes": 16722010112, "source": "host" },
  "server_budget": 4,
  "steal": { "measurable": true, "percent": 0, "verdict": "ok" },
  "throttle": { "measurable": true, "ratio": 0, "periods": 0, "throttled": 0, "verdict": "ok" },
  "verdict": "qualified"
}
```

The shape run reproduced the autotune contract on that host — including the
`saturated` classification the smoke gate has to skip on a many-core machine:

```text
shape=serial concurrency=1 req/s=26.5 class=idle      inflight=0 conc=2/4 autotune=1
shape=burst  concurrency=8 req/s=42.7 class=saturated inflight=0 conc=4/4 autotune=1
shape=serial concurrency=1 req/s=26.2 class=latency   inflight=0 conc=2/4 autotune=1
```

The allowance halves under sustained latency, returns to the cap on the burst's
first throughput window, and halves again when the burst ends — the same
transitions the smoke gate asserts.

The A/B run against itself (same binary in both arms, three interleaved samples,
200 requests per level) is the host's noise floor: replies were identical across
all three samples, and the arms stayed inside ±1% of each other at every
concurrency level.

| concurrency | base req/s | candidate req/s | base p50 | candidate p50 |
|---|---|---|---|---|
| 1 | 22.5 | 22.7 | 43.95 ms | 43.82 ms |
| 4 | 35.0 | 34.9 | 111.73 ms | 107.82 ms |
| 8 | 35.7 | 35.5 | 223.43 ms | 225.12 ms |
| 16 | 35.1 | 35.7 | 449.40 ms | 437.42 ms |

That is comfortably inside the ±5% req/s and ±10% p50 the diff enforces, so a
same-host baseline is worth recording against; the cross-host refusal itself is
covered by the tooling's tests (`TestDiffRefusesDifferingFingerprintsAndPrintsBoth`,
and the legacy baselines refused by `TestDiffRefusesTheCommittedLegacyBaselines`).

### The recorded bundle

The first dispatch that carried the bundle step (run `38434484120`, 13m26s,
`reference-arm64` on `perf-regression`, revision `ee836da`) recorded host
identity `8791c2cc689361fa`, 63 cells over tiers 1, 2 and 4, and a
`bench/fargate -mode diff` of the bundle against itself of `63 pass, 0 fail`:

```text
host-contract   pass   qualified linux/arm64: 4.00 CPUs from scheduler/affinity, server budget 4
cells-complete  pass   63 of 63 cells measured
```

No assertion is `fail` or `invalid`, so that bundle is usable as a reference.
The dispatch before it (run `38432129052`) was green while its bundle was not:
it asked for the harness's default tier list, so it requested eight CPUs from
Docker on a four-vCPU host, and the matrix stopped there.

```text
cells-complete  invalid  the run could not finish the matrix: docker run: exit status 125
                        docker: Error response from daemon: range of CPUs is from 0.01 to 4.00,
                        as there are only 4 CPUs available (63 of 84 cells measured)
```

(That detail's trailing `Run 'docker run --help' for more information` line is
elided.)

`--cpus` above the CPUs the host presents is refused, so a tier can only be
emulated up to that count: the four-vCPU rungs of the ladder (the free
`ubuntu-24.04-arm` runner, Hetzner `CAX21`, the Fargate ARM task) record tiers
1, 2 and 4, and tier 8 needs a host with eight or more CPUs. Emulating more
vCPUs than the host has would not be the tier it claims to be, so the run
records the tiers it measured and the step fails when the bundle it produced is
not a valid reference — a green job may not carry an invalid matrix.

The recorded bundle for that run is
`bundles/run.ee836da.1791911348.json` in the `emb-reference-arm64` artifact
(sha256 `9e0b04e0370ff3693a26fb722b8f272f137248ccd4ef9928c5f7605622f95d88`).
