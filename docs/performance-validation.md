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
(Δuser + Δsys) / (elapsed × cpus) ≥ 0.90   →  saturated
```

`cpus` is the fractional `registry.EffectiveCPUAllowance().CPUs` (the quota
divided by its period, bounded by affinity) and `elapsed` is the actual
monotonic time between two samples, not the nominal one-second ticker. A
half-CPU quota that burns 0.5 CPU-seconds per second is therefore fully
utilized, not 50% idle, and a sample delayed to two seconds is normalized by two
seconds. A window with completed runs but no measurable CPU accounting is
classified `unmeasured`: it holds the allowance and never claims headroom to
grow on.

Admission planning is different: it needs whole tokens, so
`CPUAllowance.Capacity()` floors the allowance to `max(1, floor(cpus))` and
bounds it by the Go scheduler. Below one CPU that rounds up to one token for
admission only; the saturation test above still sees the real fraction. A host
that presents *N* cores but delivers fewer — shared or burstable vCPU, a quota
below the presented count, steal from neighbours — is still caught by the host
contract below. Fly `shared-cpu-*` is the worked example: it enforces a
5 ms/80 ms quota per vCPU (0.0625 CPU), which plans as one admission token
while the machine bursts to four.

The quota is found from `/proc/self/mountinfo` and `/proc/self/cgroup`, not
from an assumed `/sys/fs/cgroup`: cgroup v2 and v1 `cpu` (including combined
`cpu,cpuacct` and hybrid layouts) mounts are located, a remapped mount root is
honoured, and every visible ancestor up to the mount point is read; the
tightest positive quota wins. When nothing resolves, the allowance falls back to
scheduler/affinity and the startup log carries the reason
(`CPU allowance fallback: …`).

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

The recorded cells measure wall time: the `mixed-length` cell's req/s is completed
requests over the Ruby driver's wall elapsed time (not summed request latencies),
its p50/p90/p99 are nearest-rank order statistics, and `unique-text` passes
`-r` so `redis-benchmark` really expands `__rand_int__`. `redis-benchmark` prints
no p90, so its cells omit `p90_ms` rather than reuse p95.

## The release regression driver and comparator

The cell bundles above compare one build against a recorded baseline on one host.
The release gate answers a different question: does a candidate keep a known-good
build's throughput and tails under the deployed policy? It drives each artifact
(known-good, known-pre4, candidate) through its own production startup path with
one external driver, so the server is whatever `cmd/emb` builds and the driver
only speaks RESP to it.

### The pair driver

`bench/fargate/driver` is the open-loop driver. `internal/server`'s
`BenchmarkInferencePairs` (`just bench-inference`) is a thin wrapper over the same
package; it starts an in-process server built with `registry.New()`, so it
exercises scheduling and accounting but not the governor, and its CPU and RSS
include the driver because they share a process.

- **Open loop.** Pair *i* is scheduled at `start + i/rate` whether or not earlier
  pairs finished. A scheduled pair with no free in-flight slot is `dropped`.
- **Dispatch.** `single`, `serial` (stops at the first failed leg), `parallel`
  (legs on separate connections) or `multi` (one `EMB.MULTI`).
- **Valid replies only.** Replies are decoded with `internal/resp`. A leg
  succeeds only for a bulk of exactly `dim × 4` bytes whose float32 values are all
  finite. RESP errors (`ERR busy…` is counted separately), nulls, wrong types or
  lengths, NaN/Inf, undecodable replies and transport failures are errors.
- **Exact accounting.** Every scheduled pair ends as exactly one of `success`,
  `error`, `timeout` or `dropped`, and the driver fails if `offered` is not their
  sum. Per-leg outcomes (`legs`) and error kinds (`error_kinds`) stay visible
  without double-counting pairs.
- **Bounded.** Each pair has one deadline from its wire start across all legs
  (`-request-timeout`), and the final drain waits at most `-drain-timeout` after the
  window; a pair still running then is counted as a timeout.
- **Honest rates and tails.** Throughput is successful pairs over wall elapsed
  time (at least the full window). Pair latency is reported from the scheduled time
  and from the wire start, so a late scheduler is visible; p90 is a literal
  nearest-rank value.
- **Warm and cold.** A warm phase first sends the whole warm corpus to every model;
  cold texts are unique for the process and, through a per-run nonce, across runs.
  Texts cycle short/typical/long by request index. After the pass the driver reads
  `EMB.STATS` and `EMB.INFO` deltas and fails a warm phase that missed the cache or
  a cold phase that hit it.

`EMB.INFO` initializes a lazy model. Pass `-info=false` for a cold-first-use phase
so only `EMB.STATS` (which never loads anything) is read.

### Recording a run

```bash
go run ./bench/fargate -mode drive \
  -addr 127.0.0.1:16379 -models minilm,bge -dispatch parallel -cache-state warm \
  -rate 10 -duration 120s -phase paired-parallel -cpu-limit 4 -round 1 \
  -profile bench/regression/control-profile.json -meta arm.json -bundle known-good.json
```

`-meta` names what only the person starting the arm knows: `role`
(`known-good`, `known-bad` or `candidate`), `mode`, the baseline's `approval`
(and what approved it), the `artifact` digest, runtime versions, the server's
config file and the model directories (`model.onnx` + `tokenizer.json`), which are
hashed into the bundle. `-orchestrator restarts=0,oom_kills=0,recovery_seconds=12`
supplies what the driver cannot see (restarts, OOM kills, CPU throttling,
readiness, recovery time); anything not supplied is recorded `unavailable`.
A failing run is still written to the bundle and the command exits non-zero.

Each run records the `regression` section of the bundle (`schema` 2): the role,
evidence `mode`, `approval`, `artifact` digest, `runtime` (Go/ORT version, CPU
model), `inputs` (model, tokenizer, config and corpus hashes), `observed_limits`
(the server's own `EMB.STATS` view of its CPU budget, allowance and request/
connection caps), `startup` (selected layouts and calibration metadata) and one
run per round × phase × CPU bandwidth: offered/success/error/timeout/drop counts,
throughput, both latency views, correctness, cache and admission/inference timing,
CPU, throttle and RSS, recovery, and named assertions
(`offered-accounted`, `replies-valid`, `governor-enabled`, `cache-expectation`,
`no-restart-oom`). Anything an older release does not report, such as
`cpu_admission_wait_us`, is `{"unavailable": "<reason>"}`, never zero.

The bundle's `host` is the machine running the driver. A `reference` arm is valid
only when that host passed the host contract; a `constrained-reproduction` arm
records an intentional fractional quota and is never mistaken for a qualified
reference, and also never skipped because the host is not qualified.

### The profile

`bench/regression/control-profile.json` declares the required matrix: the phases
(cold first use, preloaded startup, warm one-hot with an idle peer, paired serial,
paired parallel, `EMB.MULTI`, mixed warm/cold, sustained, burst, recovery), the
CPU bandwidths each is measured at (the sustained phase covers 0.5, 1, 1.5, 2 and
4), the declared request/health/recovery deadlines, and the required assertions.
The matrix is the profile's, never the baseline's. A profile may tighten the
default thresholds but never loosen them. The control profile's deadlines are
declared control values; the incident profile declares the deployment's own.

### The verdict

```bash
go run ./bench/fargate -mode compare -profile bench/regression/control-profile.json \
  -before known-good.json -after candidate.json
go run ./bench/fargate -mode compare -profile bench/regression/control-profile.json \
  -before known-good.json -after pre4.json -expect-fail throughput-floor   # known-bad discrimination
```

The comparator is read-only and exits 0 only on `pass`. It prints every named
check and one status:

| Status | Meaning |
|---|---|
| `invalid` | structural rejection, decided before any ratio: empty, duplicate or missing required cells; rounds that were not interleaved; incomplete runs; required assertions missing, `invalid`, `skipped` or failing on the baseline; mismatched model/tokenizer/config/corpus digests, evidence mode, host identity or server-observed limits; a zero-success control; a baseline that is not an approved `known-good`; missing artifact digests; a profile with undeclared deadlines or loosened thresholds |
| `inconclusive` | evidence too thin or noisy: fewer than 3 rounds per cell, a gated baseline run under 1000 successful pairs, any gated run under 120 s, or known-good round-to-round spread above 5% throughput / 10% latency. The ratio gates are withheld, thresholds are never widened |
| `fail` | candidate useful throughput below 95% of known-good (`throughput-floor`), pair p50/p95/p99 above 110% (`latency-p50/p95/p99`), any error, timeout or drop in a sustainable phase (`no-unexpected-loss`), a failing required assertion (`candidate-assertions`) or a recovery past its deadline (`recovery-deadline`) |
| `pass` | all of the above clear |

Ratios use the median across rounds per cell and the latency from the scheduled
time. `-expect-fail <check>` succeeds only when the verdict is `fail` on exactly
that check, so an unrelated startup failure or a missing fixture is `invalid` and
never counts as reproducing a regression.

### Harness self-verification

```bash
go test ./bench/fargate/...
```

The harness proves its own teeth: application-error replies fail `replies-valid`
(and, in a candidate arm, `candidate-assertions` and `no-unexpected-loss`), an
empty bundle fails `required-cells`, a known-good arm with zero control work fails
`control-nonzero`, a deliberate throughput collapse fails `throughput-floor`, and
a good-versus-good comparison passes. Driver tests cover a `-ERR` reply, a null
and a short blob, a stalled reply (counted as a timeout, drain bounded), and the
warm/cold cache assertions.

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

A `bench/fargate` bundle from this host is not part of the dispatched record: the
job uploads the fingerprint plus the shape and A/B logs, so the bundle-schema
clause of the end-to-end verification has no bundle to inspect. The rule it
checks — an unmet precondition is recorded `invalid` and never a pass — is
covered by the tooling's own tests.
