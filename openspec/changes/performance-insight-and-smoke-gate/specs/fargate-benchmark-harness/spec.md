# Spec Delta

## MODIFIED Requirements

### Requirement: Structured baseline capture

The harness SHALL capture a golden baseline across the workload matrix and persist it for pre/post comparison. It SHALL capture a baseline only on a host that passes the validation host contract, and SHALL record that host's fingerprint beside the captured cells, so a baseline is only ever compared with a run from the same host.

#### Scenario: Workload matrix baseline

- **WHEN** the harness completes a baseline run
- **THEN** it SHALL record req/s and p50/p90/p99 for every cell of `{vCPU, concurrency, pipeline depth, workload}` in a versioned JSON file

#### Scenario: Repeatable baseline (noise gate)

- **WHEN** the baseline is captured twice on the same host
- **THEN** the per-cell median req/s SHALL differ by no more than ±5% and p50 by no more than ±10%

#### Scenario: Host fingerprint is recorded

- **WHEN** a baseline is captured
- **THEN** the emitted file SHALL name the host's architecture, effective CPU allowance and its source, and the sampled steal and throttle verdict

#### Scenario: Unqualified host is refused

- **WHEN** a baseline is requested on a host that fails the validation host contract
- **THEN** the harness SHALL refuse to persist it as a baseline and SHALL name the failed property

#### Scenario: Baseline from another host is unusable

- **WHEN** a run is compared against a baseline whose fingerprint differs
- **THEN** the diff SHALL refuse the comparison and print both fingerprints
