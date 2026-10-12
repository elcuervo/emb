## MODIFIED Requirements

### Requirement: Container-aware thread defaults

Unset ONNX widths SHALL derive from the process CPU plan and calibration, not independently assign cores-minus-reserve to every session. The process reserve SHALL apply once. Explicit widths SHALL remain authoritative and be charged to shared admission. A reserve SHALL NOT be described as physical core isolation or a hard CPU limit.

#### Scenario: Reserve applies once

- **WHEN** an 8-CPU process uses automatic planning with the default reserve
- **THEN** the reserve SHALL be subtracted once from the shared initial budget

#### Scenario: Container limit

- **WHEN** host CPU count exceeds the process quota or affinity
- **THEN** planning SHALL use the applicable process limit instead of host totals

#### Scenario: Legacy default

- **WHEN** cpu_budget off is configured and width is unset
- **THEN** the existing legacy width rule SHALL apply

