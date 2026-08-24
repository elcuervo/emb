# inference-cpu-isolation

## Purpose

Specifies that inference (`intra_op_threads`) reserves a CPU budget by default so a busy
request path (parsing/dispatch) cannot starve ONNX, and vice versa — making constant-load
inference stable without requiring every deployment to hand-tune thread counts.

## ADDED Requirements

### Requirement: Default intra_op_threads reservation

When a model config leaves `intra_op_threads` unset, the server SHALL default it to
`max(1, cores−2)` — reserving two cores for request parsing/dispatch — rather than using
all cores. An explicit `intra_op_threads` value SHALL take precedence.

#### Scenario: Unset intra_op_threads reserves cores

- **GIVEN** a server config where `intra_op_threads` is not set for a model
- **WHEN** the model is initialized on a machine with `C` CPUs (C > 2)
- **THEN** inference uses `C−2` intra-op threads
- **AND** at least 2 cores remain available for request parsing/dispatch

#### Scenario: Explicit value wins

- **GIVEN** a model config with `intra_op_threads: 4`
- **WHEN** the model is initialized on any machine
- **THEN** inference uses exactly 4 intra-op threads

#### Scenario: Small machines floor at 1

- **GIVEN** a server running on a machine with ≤ 2 CPUs and `intra_op_threads` unset
- **THEN** inference uses 1 intra-op thread
