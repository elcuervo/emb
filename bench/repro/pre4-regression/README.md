# pre4 plain-`EMB` regression reproduction

Evidence record for group 1 of `openspec/changes/emb-inference-performance-regression`
(tasks 1.1–1.6). This directory holds the frozen identity of the incident, the arms
that are compared, and the results of each run. It is a diagnostic rig, not a
reference: the development host is not a qualified validation host
(`docs/performance-validation.md`), so nothing here may be recorded as a baseline.

## Frozen identity (task 1.1)

### Revisions — verified

```text
$ git rev-parse 'v0.4.3.pre3^{commit}' 'v0.4.3.pre4^{commit}'
a0ffc5be4661e10b7bb54f3b24f482a69c819f91   # pre3, known good
0ab054edb938aa4bdbac7236385a81187e386547   # pre4, the failing release
```

The working tree is on the `perf-regression` branch at the pre4 release line
(`VERSION` = `0.4.3.pre4`). Neither revision is built from the working tree: task 1.2
uses detached worktrees so uncommitted work is never in an arm.

### From the deployment — UNKNOWN, treated as assumptions

The production config is **not in this repository** (`config-prod.yaml` does not exist
here; the operator's copy lives with the service). Every value below is therefore an
assumption to be replaced when the deployment facts arrive, and the run records which
assumptions it used.

| Fact | Assumed value | Why that assumption |
|---|---|---|
| Task size | 4 vCPU / 8 GB, `linux/arm64` | the declared deployment target and the ladder's rung (`docs/performance-validation.md`) |
| Configured model key | `e5`-family model addressed by the app as `Emb[:hyperclusters]` | the incident path is `E5.encode`; the key itself is unverified |
| `preload` | unset (false) | the repo's own `config.yaml` leaves it unset; the fargate harness sets it true — this is exactly the divergence the reproduction must cover |
| `batching` | unset (1 ms window, `max_batch` 32, `max_batch_tokens` 16384) | `internal/config/config.go` defaults |
| `cpu_budget` / `cpu_reserve` | unset (`auto`) | defaults; pre3 has no such key at all (verified: `git show a0ffc5b:internal/config/config.go` has no match) |
| `calibrate` | unset (enabled) | pre4 default |
| `workers` / `intra_op_threads` | unset | so pre3 derives `autoTuneWorkers` / cores−2 and pre4 derives 1 session / the model share |
| `max_concurrent_requests` | unset (0 = unlimited) | `internal/config/config.go` |
| `lazy:` mode, pool size, timeouts | unknown | client-side; the eager path is unchanged between the revisions |
| Incident-side logs | unavailable | `EMB.STATS`/boot logs from the window would collapse the hypothesis ranking |

### Local host — diagnostic only

```text
darwin/arm64, 10 CPUs, 25769803776 bytes RAM
```

Not a qualified host: wrong platform for the deployment target and steal is not
measurable on darwin. Derived plan on this host, defaults, single model:
capacity 10 → reserve 2 → **budget 8**.

### Models — verified digests

| Model | File | sha256 | Bytes |
|---|---|---|---|
| `e5-small` (E5-family proxy) | `models/e5-small/model.onnx` | `b2a43b66f7f9b6f29643a21340ad7c03d2d91e6bd5d43429a77799a9ee880eb0` | 133093467 |
| | `models/e5-small/tokenizer.json` | `d241a60d5e8f04cc1b2b3e9ef7a4921b27bf526d9f6050ab90f9267a1f9e5c66` | 711396 |
| `minilm` (secondary) | `models/minilm/model.onnx` | `759c3cd2b7fe7e93933ad23c4c9181b7396442a2ed746ec7c1d46192c469c46e` | 90387606 |
| | `models/minilm/tokenizer.json` | `da0e79933b9ed51798a3ae27893d3c5fa4a201126cef75586296df9b4d2c62a0` | 711661 |

Runtime: ONNX Runtime `v1.27.0`, libtokenizers `v1.27.0` (`.github/versions.env`), Go
1.26, everything inside `nix develop`.

**The production model is not available here.** `e5-small` stands in for the E5 family
and is labelled a proxy; every number in this directory is therefore diagnostic and
cannot be quoted as the incident's magnitude.

### Client

`gems/emb` `read_timeout`/`write_timeout` = 10 s, `reconnect_attempts` = 0, pool = 5
per URL; the eager path is byte-identical between the revisions. The reproduction's
direct-RESP arm removes the client from the measurement; the Proxy arm is run only when
the deployed `lazy:` mode is known.

## Arms

Each arm runs the same model, config and offered load, and differs only in the
dimension named. Configs are written per arm (`cfg/`) because pre3 does not accept
pre4's keys (`cpu_budget`, `cpu_reserve`, `calibrate`).

| Arm (`run-matrix.sh`) | Server | Config | What it isolates |
|---|---|---|---|
| `pre3-default` | pre3 | `batched-default.yaml` | known-good baseline, batched (both revisions: one batcher session) |
| `pre4-default` | pre4 | `batched-default.yaml` | the failing default: same single session, `spinning=off`, first request pays 3.6 s of calibration |
| `pre3-pool` | pre3 | `pool-default.yaml` (`batching.timeout: 0`) | unbatched pre3: `autoTuneWorkers` sessions |
| `pre4-pool` | pre4 | `pool-default.yaml` | unbatched pre4: **one** session under the policy |
| `pre4-budget-off` | pre4 | `budget-off-pre4.yaml` | the whole policy family reverted (`cpu_budget: off`) |
| `pre4-nocalib` | pre4 | `no-calibrate-pre4.yaml` | planning and gate kept, calibration removed |
| `pre3-gate` | pre3 | `gate-pre3.yaml` (4 workers × 3 threads, unbatched) | layout held, **no gate** |
| `pre4-gate` | pre4 | `gate-pre4.yaml` + `cpu_budget: 3` | the same layout **with** the gate: width 3 = capacity 3, so runs are admitted alone |
| `pre3-preload` / `pre4-preload` | both | `batched-preload.yaml` | model load (and calibration) at boot instead of on the first request |

## Rig

| File | Role |
|---|---|
| `drive.py` | dependency-free RESP driver; modes `sweep` (latency + throughput per concurrency level), `probe` (single-request latency while 128-way load runs), `hol` (one wide command then narrow ones) |
| `run-arm.sh` | starts one binary with one config, records startup time, the effective layout (`EMB.INFO`), `EMB.STATS` before/after, runs the driver modes, stops the server |
| `run-matrix.sh` | runs every arm below and prints the summary table |
| `cfg/*.yaml` | one config per arm; pre3 arms carry only pre3 keys, because **unknown YAML keys are silently ignored** (`internal/config` does not enable `KnownFields`), so a stray pre4 key in a pre3 arm would silently measure the wrong thing |

Both revisions are built in detached worktrees with `models/` symlinked read-only, so
neither the working tree nor the model is mutated:

```bash
mkdir -p /tmp/emb-repro
git worktree add --detach /tmp/emb-repro/pre3 a0ffc5be4661e10b7bb54f3b24f482a69c819f91
git worktree add --detach /tmp/emb-repro/pre4 0ab054edb938aa4bdbac7236385a81187e386547
for rev in pre3 pre4; do ln -sfn "$PWD/models" "/tmp/emb-repro/$rev/models"; done
nix develop --command bash -c 'cd /tmp/emb-repro/pre3 && just build && cd /tmp/emb-repro/pre4 && just build'

# EMB_ORT_LIB is required on darwin: macOS SIP strips DYLD_* from any script
# started through `#!/usr/bin/env bash` (a nested `bash -c` keeps it, a script
# does not), so the dev shell's ORT path never reaches run-arm.sh by itself.
nix develop --command bash -c 'EMB_ORT_LIB=${DYLD_LIBRARY_PATH%%:*} \
  bench/repro/pre4-regression/run-matrix.sh --out /tmp/emb-repro/results'
```

## Method

1. Start the arm's server on a free port with its generated config; wait for
   `EMB.READY`.
2. Record the effective layout from `EMB.INFO <model>` and the plan from `EMB.STATS`
   before load.
3. Drive a burst of concurrent plain `EMB` requests (open-loop, fixed arrival rate and
   a fixed concurrency ceiling) and record per-request latency.
4. Record `EMB.STATS` during load and after the queue drains.
5. Stop the server; write the arm's JSON/text evidence beside this file.

Phases mirror the incident: cold start → warm serial → concurrent burst → overload →
recovery, plus a large-command-followed-by-small-requests case for head-of-line
blocking.

## Build and load verification (task 1.2)

Both revisions build in their worktrees and load `e5` with identical model settings
(`output="last_hidden_state" rank=3`, `pooling=mean`, `dim=384`, `batching=1ms/32/16384`).
What differs is the layout the server derives, and when it derives it:

| | pre3 (`0.4.3.pre3`) | pre4 (`0.4.3.pre4`) |
|---|---|---|
| CPU plan log | *(no such line)* | `CPU inference budget: allowance=10.00 source=scheduler/affinity scheduler=10 affinity=10 capacity=10 reserve=2 budget=8` |
| `EMB.INFO` layout | `workers=1`, no `cpu_*` fields | `workers=1`, `cpu_budget=8`, `cpu_thread_share=8`, `intra_op_threads=8`, `allow_spinning=0` |
| Calibration | *(none)* | `CPU calibration workers=1 intra_op_threads=8 source=calibrated reason= elapsed=3.615s` |

Two things this verification already shows, both of which the reproduction has to carry:

1. **The layout is decided on the first request, not at readiness.** `EMB.READY` answered
   immediately; the model's load and pre4's 3.6 s calibration then ran under
   `entry.once.Do` on the first plain `EMB` (mechanism M6). Readiness is gated on
   `modelCount > 0` only.
2. **That wait is invisible in the server's own stats.** The two requests issued during
   the run report `avg_latency_us` 1764 (pre3) / 2375 (pre4) and `embed_run_us` 3529 /
   4751, i.e. the pool's aggregate excludes the initialization the client actually
   waited for (the measurement blind spot in `proposal.md`).

On this 10-CPU host the *batched default* pair lands on the same session count and the
same width (8), so the pair is not expected to separate there — the separating arms are
the pool-mode pair and the gate pair.

## Results

`EMB_ORT_LIB=… run-matrix.sh`, darwin/arm64 10 CPUs, `e5-small`, 200 requests per level,
128-way background for the probe. `cold first` is the first plain `EMB` after
`EMB.READY`; `ready` is process start → `EMB.READY` OK.

| arm | layout (from `EMB.INFO`) | c128 req/s | c128 p99 | probe p50 | cold first | ready |
|---|---|---|---|---|---|---|
| `pre3-default` | 1 batcher session × 8 threads (cores−2), spinning default | 1223.1 | 92.9 ms | 93.5 ms | 0.26 s | 0.58 s |
| `pre4-default` | 1 batcher session × 8 threads (**calibrated**), `spinning=off` | 1263.8 | 93.6 ms | 89.3 ms | **3.86 s** | 0.58 s |
| `pre3-preload` | as pre3-default, model load at boot | 1218.2 | 93.7 ms | 91.5 ms | 0.002 s | 0.55 s |
| `pre4-preload` | as pre4-default, load + calibration at boot | 1243.8 | 94.7 ms | 89.4 ms | 0.003 s | **4.18 s** |
| `pre4-budget-off` | legacy sizing, no gate, no calibration | 1204.3 | 94.2 ms | 91.0 ms | – | 0.58 s |
| `pre4-nocalib` | 1 batcher session × 8 threads (raw share), calibrated: off | 1246.8 | 95.0 ms | 89.5 ms | – | 0.58 s |
| `pre3-pool` | **10 sessions × 8 threads** (`autoTuneWorkers` × cores−2) | 253.6 | 483.4 ms | 487.5 ms | – | 0.58 s |
| `pre4-pool` | 8 sessions × 1 thread (calibrated) | 1036.7 | 113.2 ms | 113.1 ms | – | 0.58 s |
| `pre4-nocalib-pool` | **1 session × 8 threads** (policy default, calibration off) | 379.5 | 325.0 ms | 330.9 ms | – | 0.58 s |
| `pre3-gate` | 4 sessions × 3 threads, **no gate** | 728.3 | 162.0 ms | 166.9 ms | – | 0.56 s |
| `pre4-gate` | the same 4 × 3 layout, **gate capacity 3, width 3** | **405.7** | **303.5 ms** | **309.8 ms** | – | 0.58 s |
| `pre3-gate-spinoff` | as `pre3-gate`, `allow_spinning: false` | 879.0 | 135.4 ms | 138.0 ms | – | 0.56 s |
| `pre4-gate-spinon` | as `pre4-gate`, `allow_spinning: true` | 335.2 | 365.2 ms | 369.2 ms | – | 0.57 s |

### What separates, and what does not

1. **The frozen pair does not separate here.** `pre4-default` ≈ `pre3-default`
   (1263.8 vs 1223.1 req/s; probe p50 89.3 vs 93.5 ms; p99 93.6 vs 92.9 ms). On a
   10-CPU host the default derivation and the calibration both land on 8 threads, so
   the batched default path is the control, not the regression. `cpu_budget: off`
   reproduces pre3 as expected, which localizes the *policy family* but not a
   mechanism (it changes sizing, the gate and calibration together).
2. **The gate serializes runs, and that costs throughput.** The decisive measurement
   is the gate's own token: with the layout held identical (4 sessions × 3 threads,
   `cpu_budget: 3`) `cpu_peak_tokens` equals **3 = capacity = the run's width**, i.e.
   the gate never had two runs in flight, and the load curve is flat
   (`c1` 402.1 ≈ `c128` 405.7 req/s — the serialization fingerprint). Matched for
   spinning (see 3 below), aggregate throughput is **879.0 → 405.7 req/s (2.17×)** and
   the single-request-under-load probe 138.0 → 309.8 ms. The per-run wait is short
   (below the 20 ms attribution threshold) because one `e5-small` run is ~2.5 ms; the
   loss is concurrency, not queue depth. Mechanism: `width == capacity` makes
   `canAdmit` require an empty gate (`permit/budget.go:55-56`) and the FIFO head rule
   (`:36`) parks everything behind it. This is the D4 defect measured.
3. **ORT spinning is not the cause; the policy's choice is the better one here.**
   The pre3–pre4 gate comparison also differs in `allow_spinning` (pre4 forces it off
   under the CPU policy), so both directions were measured with the layout and budget
   fixed:

   | arm | `allow_spinning` | c128 req/s | probe p50 |
   |---|---|---|---|
   | `pre3-gate` | default (on) | 728.3 | 166.9 ms |
   | `pre3-gate-spinoff` | off | **879.0** | **138.0 ms** |
   | `pre4-gate` | off (forced) | 405.7 | 309.8 ms |
   | `pre4-gate-spinon` | on | 335.2 | 369.2 ms |

   Spinning *off* is faster in both revisions on this host, so the policy's forced-off
   default is not a regression — which overturns the plan to restore it (design D5,
   task 4.2) and is recorded there.
4. **Cold start is the largest measured regression.** The first plain `EMB` takes
   **3.86 s** on pre4 versus **0.26 s** on pre3 (14.8×): pre4 runs `calibrateLayout`
   inside `entry.once.Do` on the request path, and `EMB.READY` answered 0.58 s in,
   before any layout existed. With `preload: true` the same work moves to boot and
   readiness takes **4.18 s** instead of 0.55 s. Per-request attribution (task 1.5)
   splits that first request: `init=3.875s embed=3ms total=3.878s`, so the entire
   stall is the model-load-plus-calibration segment inside `GetOrInit`; the server's
   own `avg_latency_us` (2375 µs) does not contain it.
5. **The "concurrency collapse" does not reproduce as a regression on this host -- but
   the *uncalibrated* policy default does.** Calibration rescues pool mode
   (8 sessions × 1 thread: 1036.7 req/s). With `calibrate: false` the policy default is
   **1 session × 8 threads** (379.5 req/s, p99 325.0 ms) — the strictly serial layout,
   2.7× worse tail than pre3-default. Conversely pre3's own oversubscription is
   substantial (10 sessions × 8 threads = 253.6 req/s, p99 483.4 ms), i.e. the CPU
   *policy* is a net win here; what regresses is the layout when calibration does not
   run (fallback, `calibrate: false`, or a share of 1).

### Per-request attribution (task 1.5)

Temporary instrumentation, identical in both worktrees, one line per request whose
total exceeds the threshold: `ATTRIB model=… init=… embed=… total=…` around
`GetOrInit` and `embedTexts` (`internal/server/server.go`), plus `ATTRIB admit wait=…
width=… capacity=… used=… peak=…` inside `budgetedSession.Run`
(`internal/registry/cpu_budget.go`, pre4 only — pre3 has no gate, so that segment does
not exist there; the init/embed instrumentation is the identical part).

- **Sum check.** Cold request: `init=3.875s`, `embed=3ms`, `total=3.878s`, against the
  client's measured 3.861 s for the same request — the segments are taken from one
  clock by construction (`embed = total − init`), and they reconcile with the client's
  number to within **0.5 %** (the client stops at the first reply byte, the server
  after the write returns). Stated tolerance: ±1 %.
- **Segments the task lists that are not separable, and why.** Tokenization and ONNX
  entry/exit are not split out: in batched mode the batcher's `flush` runs tokenize +
  ONNX + pooling inside one worker call (`internal/pipeline/batcher.go:138`), and in
  pool mode each worker's `process` does the same (`pool.go:78`), so a per-request
  boundary between them needs ORT-level hooks rather than timestamps; the local pool
  wait is reconstructed from the difference between the client total and
  `init+embed` when it matters. This is the blind spot group 6 closes permanently.

## Decision (task 1.6)

**Mechanism that fired — cold-start initialization on the request path (measured).**
The first plain `EMB` pays model load plus `calibrateLayout` inside `entry.once.Do`:
**3.878 s versus pre3's 0.234 s (16.6×)**, attributed per request as
`init=3.875s embed=3ms total=3.878s`. `EMB.READY` answered 0.58 s in, before any layout
existed; with `preload: true` the same work moves to boot and readiness becomes 4.18 s
instead of 0.55 s. On a 4-vCPU task with the production model — larger than `e5-small`,
up to 12 candidate sessions, and a 4 s start-deadline that an in-flight candidate may
overrun — this is the only measured mechanism whose magnitude can reach the incident's
~10 s. It is also invisible to the server's own aggregate, so it matches an incident
that had to be reconstructed from the client side.

**Mechanism demonstrated, but masked by the deployed shape — the gate's exclusive
admission.** `cpu_peak_tokens` equals capacity at a fixed 4 × 3 layout, and aggregate
throughput falls 2.17× (879.0 → 405.7 req/s) with spinning matched. On the declared
4-vCPU shape (capacity 4, budget 3, one model → share 3 → width == capacity) the
batcher already serializes to one run, so the gate adds no further loss there; it
becomes a latency mechanism whenever more than one run can be in flight — multiple
models, script or image co-tenancy, or a pinned pool.

**Ruled out by measurement.** The pool-mode concurrency collapse (calibration restores
8 sessions × 1 thread, 1036.7 req/s) and the policy's forced `allow_spinning: false`
(spinning off is faster in both revisions: 879.0 vs 728.3 req/s at c128).

**Inconclusive for the frozen pair on this host.** The batched default pair does not
separate (1263.8 vs 1223.1 req/s), so the pair's verdict is *not separated here* and the
missing measurement is named: a qualified host running the deployed shape (4-vCPU
budget, the deployed `preload`, the production model).

**Decisions taken, with the alternative each rejected.**

| Decision | Support | Alternative rejected |
|---|---|---|
| D4: cap an admitted run's width below the budget when the budget admits more than one run (reserve one unit), so `width == capacity` exclusivity cannot occur | `peak == capacity` with only one run ever in flight; 2.17× aggregate loss at a fixed layout | pinning `workers`/`intra_op_threads` as the fix — it hides the cold start and leaves the admission queue unbounded |
| D5: keep calibration's session count and leave ORT spinning at the policy default | calibrated pool 8 × 1 = 1036.7 req/s; spinning off faster than on in both revisions | restoring spinning (882 vs 728) and forcing two sessions (not needed for the measured loss) |
| Calibration off the request path, readiness gated on the decided layout (group 5) | 16.6× cold-request regression, and readiness answering before the layout exists | documenting `preload: true` as the operational fix — it moves the stall to readiness (4.18 s) instead of removing it |
| Admission queue bound = budget × 2 waiters plus the existing `ERR busy`, finalized by the tier-2 burst test | no measured queue here (one catch: this host never queued) | an unbounded queue with a deadline — the deadline bounds the caller but not the load |
| Differential host: the free `ubuntu-24.04-arm` runner | it qualified (host contract) in the archived change's record; this laptop fails the platform and steal properties | this laptop, whose numbers are diagnostic only |

**Still open, and why it does not block groups 2–5.** The deployed model map, `preload`,
`lazy:` mode and incident-window logs would bind the magnitude, not the mechanism; the
queue bound needs the operator's latency SLO. Nothing measured contradicts the fixes
those groups implement.

## Fix verification (tasks 3.2/4.1)

The arm that measured the D4 defect (`cfg/budget3-pool.yaml`: policy defaults at
`cpu_budget: 3`, unbatched, calibration off — the layout that produced
`cpu_peak_tokens == capacity`), re-run against the fixed tree with the same config,
model and load. The only difference is the code.

| | pre4 | fixed |
|---|---|---|
| layout (`EMB.INFO`) | 1 worker × 8 threads, share 3 | **2 workers × 1 thread**, share 3 |
| `cpu_peak_tokens` in a budget of 3 | **3** — one run at a time | **2** — two runs overlap |
| c128 | 379.5 req/s, p99 325.0 ms | **545.8 req/s, p99 223.7 ms** |
| single request under 128-way load | p50 330.9 ms | **p50 230.1 ms** |
| c1 | 374.2 req/s | 309.7 req/s |

+44 % throughput and −31 % tail latency on the arm that reproduced the exclusivity, at
the cost of single-run speed (width 3 → 1: c1 374 → 310 req/s). That is the trade the
decision makes deliberately: a burst pays less, an isolated request pays slightly more.
The derived layout is now what `EMB.INFO`'s `intra_op_threads` reports (the share stays
visible as `cpu_thread_share`), so an exclusive layout is visible instead of implied.

### Missing cells, and what blocked each

| Cell | Blocker |
|---|---|
| Ruby `E5.encode`/`Proxy` arm | the deployed `lazy:` mode and model map are unknown (proposal open questions 1 and 3); `gems/emb`'s eager path is byte-identical between the revisions, so the arm cannot add information without them |
| determinism-degraded branch | `e5-small` passes the probe in both revisions (`batch_determinism=passed`); no available model fails it, so the branch is exercised by its layout consequence (`batching: timeout: 0` = the pool arms) |
| production model | not in this repository; `e5-small` is a labelled proxy (see the identity section) |

### One-factor localization (task 1.4)

| Switch | Arms | What it changed |
|---|---|---|
| policy family | `pre4-default` → `pre4-budget-off` | sizing, gate and calibration together (≈ pre3-default: 1204.3 vs 1223.1 req/s) — localizes the family only |
| calibration | `pre4-default`/`pre4-pool` → `pre4-nocalib`/`pre4-nocalib-pool` | boot work removed; pool mode falls to the serial policy default (1036.7 → 379.5 req/s) |
| gate | `pre3-gate-spinoff` → `pre4-gate` | nothing but the gate (same 4 × 3 layout, both spinning off): 2.17× throughput (879.0 → 405.7), p99 135.4 → 303.5 ms, `cpu_peak_tokens` 3 = capacity |
| spinning | `pre3-gate` → `pre3-gate-spinoff`, `pre4-gate` → `pre4-gate-spinon` | spinning on costs throughput in both revisions (728.3 vs 879.0; 335.2 vs 405.7) — the policy's forced-off default is the better choice here |
| preload | `pre4-default` → `pre4-preload` | moves the 3.86 s request-path stall to a 4.18 s readiness delay |
| unbatched path | `*-default` → `*-pool` | the pool layout: pre3 10 × 8, pre4 calibrated 8 × 1 or uncalibrated 1 × 8 |
