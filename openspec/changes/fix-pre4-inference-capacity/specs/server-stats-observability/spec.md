# Spec Delta

## ADDED Requirements

### Requirement: CPU admission pressure is observable

EMB.STATS SHALL report cumulative `cpu_admission_wait_us`, current `cpu_admission_waiters`, and cumulative `cpu_admission_timeouts`. Model-path timing SHALL distinguish local dispatch wait, global admission wait, and native execution time; native run duration SHALL exclude global admission waiting. Counters SHALL preserve existing fields and exact RESP array framing.

#### Scenario: Gate contention without native execution
- **WHEN** a run waits at the CPU gate before native execution starts
- **THEN** admission wait and waiter observations SHALL reflect it
- **AND** that waiting duration SHALL NOT be charged as native execution time

#### Scenario: Wait expires
- **WHEN** an admission wait expires
- **THEN** the timeout counter SHALL increment once and the waiter gauge SHALL decrease
- **AND** canceled work SHALL NOT increment successful native-run counters

#### Scenario: Protocol framing and compatibility
- **WHEN** old or new clients read stats with the added fields
- **THEN** existing fields SHALL remain present and the declared RESP element count SHALL match the actual response

### Requirement: Calibration and serving path state is observable

EMB.INFO SHALL expose calibration source, limiting or fallback reason, total duration, candidate ceiling, selected layout, and initialization state for each configured inference path. Observations SHALL distinguish planned and materialized resources without initializing lazy paths. Filesystem resource discovery SHALL NOT occur while rendering stats.

#### Scenario: Memory-limited selection
- **WHEN** calibration retains a smaller layout because resident expansion cannot fit
- **THEN** the model's observed calibration reason SHALL identify memory as the limiting resource

#### Scenario: Lazy path observation
- **WHEN** a model has initialized script resources but no embedding pool
- **THEN** its reported path states SHALL distinguish those conditions without creating an embedding pool

#### Scenario: Calibration disabled
- **WHEN** calibration is disabled while CPU admission remains enabled
- **THEN** observations SHALL identify the non-calibrated layout source and SHALL NOT label it as measured calibration
