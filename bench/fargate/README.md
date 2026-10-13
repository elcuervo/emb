# fargate benchmark harness

Reproducible Fargate-shaped benchmark harness for `emb` on ARM64/Graviton CPU.

## Purpose

All project benchmarks in `BENCHMARK.md` were measured on Apple M1 Pro (macOS).
Production targets `linux/arm64` Fargate tasks. This harness replays the server
inside a `linux/arm64` Docker container bounded by `--cpus`/`--memory` matching the
Fargate vCPU tiers (1/2/4/8), drives it with `redis-benchmark` and a pure-Ruby
mixed-length RESP driver, and emits one evidence bundle per run: the host
fingerprint, the revision, the model digests, the configuration, the per-cell
metrics, and the outcome of every assertion it ran. A baseline recorded on a
qualified host is the reference a candidate bundle is diffed against — same host
only.

## Host fingerprint and "gold reference"

A run records the validation host contract's verdict (`just bench-host-check`)
in its bundle. The **gold reference** is a host that passes that contract: the
deployment platform (`linux/arm64`), an effective CPU allowance within 10% of
the presented CPUs, measurable steal under 1%, and reported throttling under
10%. Apple Silicon runs the `linux/arm64` image natively (no emulation) — good
for iteration, but not the deployment platform and steal is unmeasurable there,
so it is **not** gold.

The bundle's `host` block *is* the fingerprint: `os`, `arch`, `presented_cpus`,
`allowance{cpus,source,quota,scheduler,affinity}`, `memory_limit_bytes`,
`server_budget`, `steal_verdict`, `throttle_verdict`, plus the contract's
`identity` digest, `gold`, and `failed_property`/`failed_properties`. `identity`
hashes the stable host shape only; the sampled steal and throttle verdicts are
recorded beside it but deliberately excluded, because they follow the load on
the host rather than identify it.

**Refusal rules** (both exit non-zero and write nothing):

- `-mode baseline` refuses an unqualified host, printing the verdict and the
  failed property. Only a run that passes the contract may become a reference.
- `-mode diff` refuses two bundles whose `host.identity` differs, printing both
  fingerprints field by field. A bundle with **no** fingerprint (the two
  committed `baseline.<sha>.json` files predate the fingerprint) is also
  refused, never silently compared: re-record it with `just
  bench-fargate-baseline` on a qualified host.

`-mode run` still records on an unqualified host, marked `"gold": false`; such a
bundle is usable as a `-after` candidate as long as the fingerprint identity
matches.

## CPU quota emulation

Every tier container runs with `--cpus <tier>` **and** `GOMAXPROCS=<tier>`
(container env). The env pin matters on hosts where the container CPU quota is
invisible to Go (Docker Desktop / OrbStack on macOS): there `runtime.GOMAXPROCS`
otherwise reports the host core count and the measured tiers would not match the
gold host, where Go honors the task cgroup quota. The env pin makes tier behavior
deterministic on every host.

Note that pinning GOMAXPROCS to the tier does **not** make throughput monotonic
in the tier: `intra_op_threads` defaults to `cores−2` (6 threads at tier 8), and
for small models (dim ≤ 768) more than ~2 intra-op threads loses to thread sync —
measured: tier8 c1 ≈ 140 req/s (6 threads) vs ≈ 550 at tier 2 (1 thread), and an
isolated 1-thread tier-8 run hits ≈ 680 req/s. On the gold host the same default
applies (cgroup quota → tier cores), so the non-monotonic curve is real deployment
behavior, not a harness artifact; operators should set `intra_op_threads`
explicitly (≤ 2 for small models) in production configs.

## Workloads

| Workload       | Driver             | Purpose                                                       |
|----------------|--------------------|---------------------------------------------------------------|
| `fixed-length` | `redis-benchmark`  | uniform short texts (~8 tokens), batching efficiency baseline  |
| `unique-text`  | `redis-benchmark`  | unique texts via `__rand_int__`, expanded because the run passes `-r` (cache-cold, high token churn)|
| `mixed-length` | `bench/fargate/load.rb` | 80% short / 20% long texts, interleaved — padding-waste probe |
| `cache-hit`    | `redis-benchmark`  | server launched with `-cache auto`, warmed once               |

## The bundle

One JSON bundle per recorded run:

| Field | Contents |
|---|---|
| `revision` | git sha of the measured tree |
| `created` | UTC RFC3339 timestamp |
| `host` | the fingerprint (above): the shape, `identity`, `gold`, failed properties |
| `model` | `name`, and `onnx`/`tokenizer` `{path,bytes,sha256}` of the files actually measured |
| `config` | `platform`, `model`, `max_length`, `cpus`, `clients`, `pipelines`, `count_per_cell`, `req_gate`, `p50_gate` |
| `cells` | `{tier,workload,clients,pipeline,req_s,p50_ms,p90_ms,p99_ms,padding_efficiency}` per measurement; `p90_ms` is absent when the tool prints no literal p90 |
| `assertions` | `{name,outcome,detail}` with outcome `pass`/`fail`/`invalid`/`skipped`/`inconclusive` |
| `schema`, `regression` | release-comparator evidence (below); absent from cell bundles |

Assertions recorded: `host-contract` (passes when the host is qualified, fails
naming the property otherwise) and `cells-complete` (passes when every requested
cell produced a measurement, `invalid` when the run could not finish the
matrix). `invalid` is only ever an unmet precondition and `skipped` only ever an
assertion the run could not exercise — neither is a pass. A bundle that fails
the contract stays valid evidence of a `run`, but is not a reference.

## Metrics per cell

`{vCPU tier} × {clients} × {pipeline} × {workload}` → req/s, p50/p90/p99 ms, plus
**padding efficiency** for mixed-length (real tokens / processed token-slots,
computed with the real tokenizer over a `max_batch` window). Each cell is the
median of 3 runs. `mixed-length` req/s is completed requests over the driver's wall
elapsed time and its percentiles are nearest-rank order statistics; neither is
derived from summed request latencies or approximated from a neighbour.

**Diff gates:** inference-bound cells (<5000 req/s) require req/s ≥ −5% and
p50 ≤ +10%. Saturated cells (≥5000 req/s, e.g. cache-hit) finish in
sub-millisecond windows where timer quantization dominates, so they gate on
p50 ≤ +10% only. The diff refuses to run at all unless both bundles carry the
same host fingerprint.

## Usage

```
nix develop
just bench-fargate-baseline          # qualified host only: bench/fargate/baseline.<sha>.json
just bench-fargate                   # any host: bench/fargate/out/run.<sha>.<ts>.json
just bench-fargate-diff <baseline.json> <candidate.json>   # same-host per-cell PASS/FAIL vs tolerances
```

`bench-fargate-diff` passes its first argument as `-before` (the recorded
baseline) and its second as `-after` (the candidate); the diff exits non-zero
when a cell regresses past the tolerances, or when the two fingerprints differ.

`docker` (and `git`) are not part of the `nix develop` profile, so the harness
resolves them with absolute-path fallbacks (`/usr/local/bin`, `/opt/homebrew/bin`,
`/usr/bin`, `~/.local/bin`). Set `PATH` or add `/usr/local/bin/docker` (etc.)
yourself if Docker lives somewhere else.

Flags for `go run ./bench/fargate` (see `go run ./bench/fargate -h`):
`-cpus -clients -pipeline -platform -count -model-dir -model -max-length -image
-out -mode (run|baseline|diff|drive|compare) -before -after -req-gate -p50-gate
-mixed-count -short-len -long-len -short-ratio -max-batch -batching-timeout
-max-batch-tokens -skip-build -keep`. `drive` and `compare` add `-addr -models
-dispatch -cache-state -rate -duration -request-timeout -drain-timeout
-max-in-flight -dim -phase -round -cpu-limit -profile -bundle -meta -info
-orchestrator -expect-fail -seed`.

## Release regression driver

`-mode drive` runs one open-loop pass of the pair driver (`bench/fargate/driver`)
against an external, already-running server and appends it to a regression
bundle; `-mode compare` reads two bundles under a profile and prints a named
verdict (`pass`, `fail`, `inconclusive`, `invalid`). The driver is also what
`internal/server`'s `BenchmarkInferencePairs` calls, so the benchmark is a thin
wrapper. The control profile lives in `bench/regression/` (a server config and
`control-profile.json`).

```
go run ./bench/fargate -mode drive -addr 127.0.0.1:16379 -models minilm,bge \
  -dispatch parallel -cache-state warm -rate 10 -duration 120s \
  -phase paired-parallel -cpu-limit 4 -round 1 \
  -profile bench/regression/control-profile.json -meta arm.json -bundle known-good.json
go run ./bench/fargate -mode compare -profile bench/regression/control-profile.json \
  -before known-good.json -after candidate.json
go test ./bench/fargate/...          # driver, comparator and the self-verification cases
```

Every scheduled pair is classified exactly once (`success`, `error`, `timeout`,
`dropped`); a success needs a valid, finite float32 embedding of the expected
dimension. The full contract (dispatch modes, deadlines, warm/cold counters, the
`arm.json` metadata, bundle fields, thresholds, `-expect-fail`) is in
[docs/performance-validation.md](../../docs/performance-validation.md).

## Layout

```
bench/fargate/
  main.go          # orchestrator: host contract, build image, run tiers, workloads, bundle/diff
  main_test.go     # bundle schema, refusal rules, tolerance math (no Docker/model/network)
  workload_test.go # redis-benchmark/mixed-workload fixes: wall-clock req/s, literal p90, __rand_int__
  load.rb          # pure-stdlib Ruby mixed-length RESP driver (latencies)
  driver/          # open-loop pair driver: scheduler, reply validation, accounting, EMB.STATS/INFO counters
  drive.go         # -mode drive: record one driven pass into a regression bundle
  evidence.go      # regression bundle section: provenance, limits, runs, named assertions
  compare.go       # -mode compare: fail-closed comparator and verdict
  compare_test.go  # rejections, verdicts and the harness self-verification cases
  baseline.*.json  # recorded baselines (pre-fingerprint files are refused by the diff)
```