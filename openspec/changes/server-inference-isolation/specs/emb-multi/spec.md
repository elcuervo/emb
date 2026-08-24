## ADDED Requirements

### Requirement: Bounded EMB.MULTI fan-out

The server SHALL cap concurrent pair-processing within a single `EMB.MULTI` to the
model's worker capacity, so request storms cannot spawn unbounded goroutines competing
for inference cores. Per-pair `nil` (MGET) semantics, result ordering, and embedding
correctness SHALL be unchanged.

#### Scenario: Large MULTI does not flood the scheduler

- **GIVEN** a running server with `minilm` registered
- **WHEN** a client sends one `EMB.MULTI` with a large number of pairs (e.g. 400) while
  many concurrent load workers send large `EMB.MULTI` commands for unknown models
- **THEN** the server remains responsive to `PING` and subsequent `EMB` requests
- **AND** concurrent pair-processing is bounded (no unbounded goroutine growth, observed
  via goroutine count / CPU)

#### Scenario: MGET semantics preserved under bounded fan-out

- **GIVEN** a running server with `minilm` registered
- **WHEN** a client sends `EMB.MULTI minilm "a" nonexistent "x" minilm "c"` where the
  "minilm" texts embed successfully and `nonexistent` is unknown
- **THEN** the response SHALL be an array in the same order with element 0 and 2 as bulk
  strings and element 1 as a null bulk string

### Requirement: Storm stability gate

The server SHALL keep inference p99 stable while the request path is under both constant
parse load and a request storm. On the reference machine, under the documented CPU
partition, the constant-load stability ratio (`p99_with_load / p99_idle`) SHALL be ≤ 1.5
and the storm ratio SHALL be ≤ 1.75.

#### Scenario: Constant-load gate passes on the reference machine

- **WHEN** the stability harness runs its constant-load mode under the CPU partition
- **THEN** the reported ratio is ≤ 1.5
- **AND** the gate fails (non-zero exit) otherwise

#### Scenario: Storm gate passes on the reference machine

- **WHEN** the stability harness runs its storm mode (many load workers sending large
  `EMB.MULTI` commands) under the CPU partition
- **THEN** the reported storm ratio is ≤ 1.75
- **AND** the gate fails (non-zero exit) otherwise
