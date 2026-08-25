## Context

The repo's only benchmark methodology (`BENCHMARK.md`) is bound to Apple M1 Pro hardware. Fargate CPU tasks on the target deployment run `linux/arm64` (AWS Graviton), where the ISA (NEON, SVE), the scheduler, and ONNX Runtime's MLAS kernels behave differently from both the current benchmark host and from x86. No performance change can be validated against the deployment target without first capturing a baseline there. The Dockerfile already builds a Fargate-compatible image and maps `TARGETARCH=arm64` → ORT `aarch64` + libtokenizers `linux-aarch64`, so a `--platform linux/arm64` build already works.

## Goals / Non-Goals

**Goals:**
- Reproducible `linux/arm64` measurement methodology committed to `bench/fargate/`
- CPU/memory limits matching the Fargate Graviton tiers (1/2/4/8 vCPU)
- Golden baseline committed for pre/post diffing
- Everything reproducible via `nix develop` + `just bench-fargate*`

**Non-Goals:**
- Deploying to an actual Fargate account (the local container replica is the validation methodology; a Graviton host is the gold reference but out of scope to provision here)
- Changing server behavior
- Simulating Fargate network latency / multi-AZ noise

## Decisions

### `docker run --cpus N --memory M` as the Fargate replica
Fargate allocates vCPU as CPU quota; a container run with `--cpus N --memory M` mirrors the quota model closely enough for relative pre/post comparisons. `taskset` partitioning inside the container is unnecessary because Docker's cpuset is the quota.

### `linux/arm64` default, `linux/amd64` overridable
The target is Graviton, so the harness builds with `--platform linux/arm64` by default. On Apple Silicon hosts the image runs natively (no emulation) and yields NEON/AMX numbers that approximate Graviton well; the harness prints a host-architecture warning so results aren't mistaken for gold. `--platform=linux/amd64` is kept for cross-checks and CI runners without ARM64.

### redis-benchmark + Ruby harness from `nix develop`
Both redis and ruby are already dev-shell inputs. The drivers run on the host under the shell against a mapped container port, keeping the runtime image minimal and the toolchain reproducible via `nix develop`.

### Golden baseline as versioned JSON
`bench/fargate/baseline.<sha>.json`; `just bench-fargate-diff <sha1> <sha2>` prints per-cell delta with PASS/FAIL per tolerance. This gives later proposals a deterministic gate ("beat baseline cell by ≥X%").

### CI (staged)
Add a GitHub Actions `linux arm64` runner job that builds the image, runs a short baseline, and gates on the noise tolerance. Staged so the harness can land and be validated locally first.

## Risks / Trade-offs

- [Docker not available on the validation host] → the server-runner behind the harness abstracts image/vm execution; documented fallback is a real Graviton instance or bare-metal ARM64 Linux.
- [Apple Silicon ≠ exact Graviton] → harness labels local-host results as "approximation" and the noise gate applies per-host; gold numbers always come from an ARM64 Linux host. Documented in BENCHMARK.md.
- [CPU-quota noise on shared hosts] → tolerances are computed as median-of-3 per cell.
- [arm64 runner availability] → CI job is staged and optional; local `nix develop` workflow is the primary validation path.

## Migration Plan

Land the harness and baseline first (`just bench-fargate-baseline`), commit the golden baseline, then gate every subsequent performance proposal (`token-budget-batching`, `async-tokenization`, `zero-copy-inference`, `int8-weight-quantization`, `ctranslate2-backend`) on pre/post diffs against it.

## Open Questions

- Which ARM64 host is the committed reference for gold numbers: a Graviton instance, an ARM64 CI runner, or Apple Silicon as approximation? (Recommended: ARM64 CI runner for automation; Apple Silicon acceptable for local iteration.)