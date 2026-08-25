## 1. Harness scaffolding

- [x] 1.1 Create `bench/fargate/` with README and layout (server runner, workload definitions, result emitter)
- [x] 1.2 Add server runner that builds the image with `--platform linux/arm64` and runs `docker run --cpus N --memory M -p <port>:6379`
- [x] 1.3 Print host architecture + platform warning when the host is not the gold reference (ARM64 Linux)

## 2. Workloads & metrics

- [x] 2.1 Define fixed-length, mixed-length, unique-text, cache-hit workloads as redis-benchmark command sets + Ruby harness for p99
- [x] 2.2 Compute req/s, p50/p90/p99, and padding efficiency per cell
- [x] 2.3 Emit versioned baseline JSON (`baseline.<sha>.json`)

## 3. just targets & docs

- [x] 3.1 Add `just bench-fargate`, `just bench-fargate-baseline`, `just bench-fargate-diff`
- [x] 3.2 Document methodology in BENCHMARK.md (Fargate/liv/arm64 section)

## 4. Validation stage (nix develop)

- [x] 4.1 In `nix develop`: `just bench-fargate-baseline` twice; verify noise gate PASS (req/s ±5%, p50 ±10% median-of-3)
- [x] 4.2 Commit golden baseline; `just bench-fargate-diff` on a clean rerun reports no drift
- [x] 4.3 (staged) Add ARM64 CI job with a short baseline gate