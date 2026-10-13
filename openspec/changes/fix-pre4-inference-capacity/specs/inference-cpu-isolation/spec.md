# Spec Delta

## MODIFIED Requirements

### Requirement: Default intra_op_threads reservation

Unset ONNX widths SHALL derive from the process CPU plan and a validated, memory-bounded resident layout, not independently assign cores-minus-reserve to every session or treat static model shares as permanent ceilings. The process reserve SHALL apply once. Explicit widths SHALL remain authoritative and be charged to shared admission. A reserve SHALL NOT be described as physical core isolation or a hard CPU limit.

#### Scenario: Unset width uses the process plan
- **GIVEN** a server config where `intra_op_threads` is not set for a model
- **WHEN** the model is initialized with `cpu_budget: auto`
- **THEN** its width SHALL come from a validated layout within the process CPU and memory ceilings, allowing use of idle peer capacity

#### Scenario: Reserve applies once
- **WHEN** an 8-CPU process uses automatic planning with the default reserve
- **THEN** the reserve SHALL be subtracted once from the shared initial budget

#### Scenario: Container limit
- **WHEN** host CPU count exceeds the process quota or affinity
- **THEN** planning SHALL use the applicable process limit instead of host totals

#### Scenario: Unset intra_op_threads reserves cores
- **GIVEN** a server running on a machine with C CPUs where C is greater than 2 and `intra_op_threads` is unset
- **WHEN** `cpu_budget: off` is configured
- **THEN** inference SHALL use C minus 2 intra-op threads
- **AND** the default SHALL leave two CPUs outside that configured inference width without claiming physical isolation

#### Scenario: Explicit value wins
- **GIVEN** a model config with `intra_op_threads: 4`
- **WHEN** the model is initialized on any machine
- **THEN** inference SHALL use exactly 4 intra-op threads

#### Scenario: Small machines floor at 1
- **GIVEN** a server running on a machine with at most 2 CPUs and `intra_op_threads` unset
- **WHEN** a model is initialized
- **THEN** inference SHALL use at least 1 intra-op thread in both budget modes
