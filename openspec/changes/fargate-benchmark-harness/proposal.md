## Why

emb targets Fargate CPU deployments on **ARM** (`linux/arm64`, AWS Graviton), but every benchmark in `BENCHMARK.md` was captured on an M1 Pro (macOS). There is no `linux/arm64` measurement methodology and no baseline in the repo, so no performance change can be validated against the real deployment shape. Every proposal in this roadmap ("token-budget-batching", "async-tokenization", "zero-copy-inference", "int8-weight-quantization", "ctranslate2-backend") defines success relative to a baseline that this harness first establishes.

## What Changes

- New `bench/fargate/` harness that reproduces the Fargate compute shape locally: builds the existing Dockerfile for **`linux/arm64`** (default — matches Graviton; `--platform linux/amd64` overridable), runs the server container under `docker run --cpus N --memory M` at the Fargate vCPU tiers (1/2/4/8), and drives it with `redis-benchmark` and the Ruby harness from the `nix develop` shell (redis and ruby are already dev-shell inputs).
- On Apple Silicon hosts the `linux/arm64` container runs **natively (no emulation)**, giving NEON/AMX numbers that are the closest local approximation of a Graviton instance — but the reference for gold numbers is an ARM64 Linux host (real Graviton or an ARM64 CI runner).
- Captures a versioned **golden baseline** (`bench/fargate/baseline.<sha>.json`) across the workload matrix:
  `{vCPU 1/2/4/8} × {concurrency 1/8/16} × {pipeline 1/8} × {fixed-length, mixed-length, unique-text, cache-hit}`.
- Per-cell metrics: req/s, p50/p90/p99, plus derived **padding efficiency** (real tokens / processed token-slots) for mixed-length workloads — the metric the batching proposals gate on.
- `just bench-fargate` / `just bench-fargate-baseline` / `just bench-fargate-diff` targets. All Go/Ruby/redis tooling runs inside `nix develop`; Docker is the only host-level dependency (used only to emulate the Fargate CPU quota).
- BENCHMARK.md gains a "Fargate (linux/arm64, Graviton)" section documenting methodology and how to diff against the baseline.
- Staged: a `linux arm64` CI job that runs a short baseline and gates on the noise tolerance.

## Capabilities

### New Capabilities

- `fargate-benchmark-harness`: reproducible Fargate-shaped benchmark methodology (`linux/arm64` container + vCPU tiers) that captures a golden baseline and diffs pre/post results with explicit tolerances.

### Modified Capabilities

(none — measurement only, no server behavior changes)

## Impact

Files: `bench/fargate/` (new harness), `justfile` (new `bench-fargate*` targets), `BENCHMARK.md`, optional `.github/workflows/ci.yml` job. No changes to `internal/`, the RESP protocol, or the Docker runtime image (the Dockerfile already maps `TARGETARCH=arm64` → ORT `aarch64` + libtokenizers `linux-aarch64`). Requires Docker on the validation host; every benchmark/build command in the change is executed from `nix develop`.

## Validation

Validation stage for the harness itself (all inside `nix develop`):

```
$ nix develop
$ just bench-fargate-baseline          # run 1 → baseline.<sha1>.json
$ just bench-fargate-baseline          # run 2 → baseline.<sha2>.json
$ just bench-fargate-diff <sha1> <sha2>
```

- **Noise gate:** per-cell median req/s within ±5%, p50 within ±10% across the two runs → **PASS/FAIL**.
- The committed golden baseline is the reference every later proposal diffs against; a proposal only ships when its cells beat the baseline within its own gates.