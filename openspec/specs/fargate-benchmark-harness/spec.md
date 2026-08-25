# fargate-benchmark-harness Specification

## Purpose
TBD - created by archiving change fargate-benchmark-harness. Update Purpose after archive.
## Requirements
### Requirement: Fargate-shaped measurement environment
The harness SHALL reproduce the Fargate CPU compute shape locally by running the server in a `linux/arm64` container with CPU and memory limits matching the Fargate vCPU tiers.

#### Scenario: vCPU tier replica
- **WHEN** the harness runs the server with `--cpus 2 --memory 4g` (2 vCPU tier)
- **THEN** the server SHALL be confined to 2 CPUs and 4 GB memory
- **THEN** reported req/s and latency SHALL reflect that tier

#### Scenario: ARM image default
- **WHEN** the harness builds the server image
- **THEN** it SHALL default to `linux/arm64`
- **THEN** it SHALL support `--platform linux/amd64` as an override

### Requirement: Structured baseline capture
The harness SHALL capture a golden baseline across the workload matrix and persist it for pre/post comparison.

#### Scenario: Workload matrix baseline
- **WHEN** the harness completes a baseline run
- **THEN** it SHALL record req/s and p50/p90/p99 for every cell of `{vCPU, concurrency, pipeline depth, workload}` in a versioned JSON file

#### Scenario: Repeatable baseline (noise gate)
- **WHEN** the baseline is captured twice on the same host
- **THEN** the per-cell median req/s SHALL differ by no more than ±5% and p50 by no more than ±10%

### Requirement: Padding efficiency metric
The harness SHALL compute and report padding efficiency (real tokens / processed token-slots) for mixed-length workloads.

#### Scenario: Mixed-length efficiency reporting
- **WHEN** the mixed-length workload runs
- **THEN** the harness SHALL report per-batch real token count, processed token-slots, and their ratio

### Requirement: Baseline diffing
The harness SHALL produce a pre/post diff against the golden baseline with PASS/FAIL per cell.

#### Scenario: Pre/post diff
- **WHEN** a proposal runs the harness against a changed build and compares to the golden baseline
- **THEN** the harness SHALL print per-cell delta and a PASS/FAIL verdict within the configured tolerances

