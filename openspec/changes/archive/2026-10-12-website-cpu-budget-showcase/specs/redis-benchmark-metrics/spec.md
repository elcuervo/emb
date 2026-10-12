## MODIFIED Requirements

### Requirement: Published benchmark results

The project SHALL publish benchmark results in BENCHMARK.md following the tidwall/redcon format, and the prose beside each result SHALL describe the behaviour of the configuration it names.

#### Scenario: BENCHMARK.md contains methodology

- **WHEN** a contributor reads BENCHMARK.md
- **THEN** they SHALL find the exact commands needed to reproduce results for each configuration
- **THEN** they SHALL find the server startup command and `redis-benchmark` invocation

#### Scenario: BENCHMARK.md contains results

- **WHEN** a contributor reads BENCHMARK.md
- **THEN** they SHALL find requests/sec numbers for each tested configuration

#### Scenario: The documented layout is the layout that runs

- **WHEN** a contributor reads the section for a configuration
- **THEN** the session count, the thread width and the policy it describes SHALL be the ones that configuration selects when it runs, including when a default policy (rather than the documented keys) determines them

## ADDED Requirements

### Requirement: A published measurement names its policy and spread

A published throughput figure SHALL name the policy that produced it (`cpu_budget: auto` or `cpu_budget: off`) and the effective layout, and SHALL be reported as the median of repeated interleaved runs with the observed spread.

#### Scenario: Repetitions decide, not a single run

- **WHEN** a throughput figure is published
- **THEN** it SHALL come from repeated runs of the same configuration, interleaved with the comparison arm, and SHALL state the spread

#### Scenario: The recipe cannot measure another process

- **WHEN** the reproduce recipe runs while another process owns the benchmark port
- **THEN** it SHALL fail with a message naming the port, rather than measure the process it found there

#### Scenario: The policy is named with the number

- **WHEN** a figure is published
- **THEN** the same section SHALL state whether it was measured under `cpu_budget: auto` or `cpu_budget: off` and what layout that policy selected
