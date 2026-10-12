# inference-cpu-isolation Specification

## Purpose
Specifies that inference (`intra_op_threads`) reserves a CPU budget by default so a busy
request path (parsing/dispatch) cannot starve ONNX, and vice versa — making constant-load
inference stable without requiring every deployment to hand-tune thread counts.

## Requirements

### Requirement: Default intra_op_threads reservation

Unset ONNX widths SHALL derive from the process CPU plan and calibration, not independently assign cores-minus-reserve to every session. The process reserve SHALL apply once. Explicit widths SHALL remain authoritative and be charged to shared admission. A reserve SHALL NOT be described as physical core isolation or a hard CPU limit.

#### Scenario: Unset width uses the process plan

- **GIVEN** a server config where `intra_op_threads` is not set for a model
- **WHEN** the model is initialized with `cpu_budget: auto`
- **THEN** its width SHALL come from the calibrated layout inside that model's share of the process budget

#### Scenario: Reserve applies once

- **WHEN** an 8-CPU process uses automatic planning with the default reserve
- **THEN** the reserve SHALL be subtracted once from the shared initial budget

#### Scenario: Container limit

- **WHEN** host CPU count exceeds the process quota or affinity
- **THEN** planning SHALL use the applicable process limit instead of host totals

#### Scenario: Unset intra_op_threads reserves cores

- **GIVEN** a server config where `intra_op_threads` is not set for a model and `cpu_budget: off`
- **WHEN** the model is initialized on a machine with `C` CPUs (C > 2)
- **THEN** inference uses `C−2` intra-op threads
- **AND** at least 2 cores remain available for request parsing/dispatch

#### Scenario: Explicit value wins

- **GIVEN** a model config with `intra_op_threads: 4`
- **WHEN** the model is initialized on any machine
- **THEN** inference uses exactly 4 intra-op threads

#### Scenario: Small machines floor at 1

- **GIVEN** a server running on a machine with ≤ 2 CPUs and `intra_op_threads` unset
- **THEN** inference SHALL use at least 1 intra-op thread in both budget modes
