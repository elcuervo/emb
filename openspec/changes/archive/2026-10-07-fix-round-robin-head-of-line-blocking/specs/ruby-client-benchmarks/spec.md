# Spec Delta

## ADDED Requirements

### Requirement: Pool tail-latency gate

The harness SHALL include a concurrent connection-pool scenario that injects occasional
slow replies and measures the p90 of end-to-end embed latency. The scenario SHALL compare
that p90 against a documented threshold derived from the work-conserving baseline
recorded in `BENCHMARK.md` and SHALL exit non-zero when the threshold is exceeded.

#### Scenario: Gate passes with work-conserving selection

- **WHEN** the concurrent pool scenario runs against a pool whose selection is
  work-conserving
- **THEN** the reported p90 SHALL be within the documented threshold of the single-command
  latency, and the gate SHALL pass

#### Scenario: Gate fails on head-of-line blocking

- **WHEN** the concurrent pool scenario runs against a pool that blocks a waiting command
  on a specific busy connection while another connection is free
- **THEN** the reported p90 SHALL exceed the documented threshold and the harness SHALL
  exit non-zero
