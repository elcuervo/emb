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

The gate prints every outcome and one summary line:

```text
emb-smoke: assertions=17 pass=16 fail=0 skipped=1
```

`lint`, `test`, and the CGo CI job fail when that line is missing, so a green
run cannot mean the gate skipped. An assertion the host cannot exercise is
reported as `skipped` with the host property that prevented it — the saturating
burst above is skipped on a host whose CPU fraction cannot reach 0.90 — and the
one-core arm (`saturation-enforced`) is where that assertion is enforced per
change instead. Verified on the development machine: 16 pass, 1 skip, 0 fail,
67 s.

## Gate self-verification

```bash
bash bench/smoke/verify-gate.sh     # ~6 min: the clean tree plus one gate run per mutant
```

A gate that cannot fail is not a gate. The sweep applies each committed
regression under `bench/smoke/mutants/` to a scratch tree — a detached worktree
at `HEAD` plus the current tree's changes, so the working tree is never touched
— and requires the gate to go red **and name the assertion that caught it**.
The catalogue is the four regressions named in the change, one patch each, with
the assertion each one must trip:

| Mutant (`bench/smoke/mutants/`) | Planted regression | Assertion that must catch it |
|---|---|---|
| `01-eager-shrink.patch` | the allowance dwell inverted: shrink made eager, so the allowance halves on the first latency window | `allowance-halved` |
| `02-saturation-out-of-reach.patch` | the saturation threshold raised out of reach, so the class can never fire | `saturation-enforced` |
| `03-cache-hits-count-as-traffic.patch` | a reply-cache hit counted as a run, so a cache-served window stops being idle | `cache-idle` |
| `04-layout-reshape.patch` | the reported session count follows the live allowance: a runtime reshape of the layout | `layout-immutable` |

Recorded sweep on the development machine:

```text
== clean tree (must pass) ==
✓ clean tree: gate green (emb-smoke: assertions=17 pass=16 fail=0 skipped=1)

== 01-eager-shrink.patch (must fail on: allowance-halved) ==
✓ 01-eager-shrink.patch: red, and named allowance-halved (emb-smoke: assertions=17 pass=15 fail=1 skipped=1)

== 02-saturation-out-of-reach.patch (must fail on: saturation-enforced) ==
✓ 02-saturation-out-of-reach.patch: red, and named saturation-enforced (emb-smoke: assertions=17 pass=15 fail=1 skipped=1)

== 03-cache-hits-count-as-traffic.patch (must fail on: cache-idle) ==
✓ 03-cache-hits-count-as-traffic.patch: red, and named cache-idle (emb-smoke: assertions=17 pass=15 fail=1 skipped=1)

== 04-layout-reshape.patch (must fail on: layout-immutable) ==
✓ 04-layout-reshape.patch: red, and named layout-immutable (emb-smoke: assertions=17 pass=15 fail=1 skipped=1)

verify-gate: every mutant was caught and the clean tree passed
```

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
4. `bench-out/reference/` is uploaded as an artifact even when the contract
   refuses the host, so the verdict is always inspectable.

The scripted model set (`models/gliner2`) is cached with `actions/cache`, the
same way `test-cgo` caches `models/minilm`.

Which rung the free `ubuntu-24.04-arm` runner actually is can only be answered
by a real dispatch. If it reports qualified, the reference tier lives there; if
it reports unqualified — steal on a shared runner is the likely property — the
tier moves down the ladder to a rented dedicated-vCPU host or to the Fargate ARM
task, and the recorded verdict below says which. The first qualified run's
verdict, and that host's baseline diffed against itself, are recorded here once
a dispatch has produced them.
