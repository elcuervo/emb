# inference-cpu-budget Specification

## Purpose
Specifies that inference capacity is planned once per process — from the container's CPU
quota, affinity/cpuset, and Go scheduler limits — and shared by every model's embedding,
scripted, image, and calibration runs through one fair admission controller. Unset session
and intra-op settings are chosen by a bounded local calibration inside each model's share
of that budget, with a documented safe fallback when no candidate can be validated.
`cpu_budget: off` restores the legacy per-model sizing policy.

## Requirements

### Requirement: Process-local CPU planning

The server SHALL resolve a process-local CPU allowance from applicable container quota, affinity/cpuset, and scheduler limits. It SHALL retain fractional allowance for accounting and derive a conservative integer admission budget. The default reserve SHALL apply once per process. Invalid reserves, non-positive/non-finite model weights, and conflicting explicit budget/reserve settings SHALL fail validation. The cpu_budget off mode SHALL restore legacy scheduling policy.

#### Scenario: Fractional quota

- **WHEN** the process allowance is 1.5 CPUs
- **THEN** admission SHALL start with one slot and stats SHALL retain 1.5 as the allowance

#### Scenario: Multiple replicas

- **WHEN** eight independent processes each have an 8-CPU allowance
- **THEN** each process SHALL plan its own budget without dividing it by replica count

#### Scenario: Explicit override above capacity

- **WHEN** an explicit budget exceeds the detected allowance
- **THEN** startup SHALL log the mismatch and keep the operator's explicit value

### Requirement: Bounded automatic calibration

Unset embedding session and thread dimensions SHALL be selected by local calibration after model metadata resolution and determinism probing. Calibration SHALL use representative short, typical, and long inputs, separate warmup from repeated trials, check correctness, and obey the process CPU share and conservative model-size memory estimate. It SHALL reject candidates whose p95 exceeds twice the single-worker baseline, choose the highest measured throughput among valid candidates, and prefer wider threads on a tie when the model batches — the trial serves a smaller batch than the coalescing pool — and lower resource use otherwise. A slow in-flight model call may exceed the nominal deadline; temporary sessions SHALL close only after calls return.

#### Scenario: Explicit settings

- **WHEN** a worker count or intra-op width is explicitly configured
- **THEN** calibration SHALL not replace that dimension

#### Scenario: Calibration cannot establish a valid candidate

- **WHEN** a fixture is invalid, candidate execution fails, or CPU accounting is unavailable
- **THEN** the server SHALL use the documented safe fallback and log the reason

#### Scenario: Probe failure

- **WHEN** batch determinism fails
- **THEN** calibration SHALL measure single-row inference and the resulting pool SHALL not retain batch-sized worker defaults

### Requirement: Shared inference admission

Embedding, scripted, image, and calibration ONNX runs SHALL share one fair process-local admission controller. Tokens SHALL be acquired immediately around actual Session.Run and released on every result path. Idle model capacity SHALL be borrowable. Explicit work wider than the budget SHALL run alone with a warning. The controller SHALL report admission wait separately from preprocessing, local pool wait, and run time. Tokens are an admission bound, not an OS CPU limit.

#### Scenario: Concurrent models

- **WHEN** multiple models execute concurrently
- **THEN** their admitted work SHALL share the process budget and preserve fair progress

#### Scenario: Failed run

- **WHEN** an ONNX run returns an error
- **THEN** all admission capacity owned by that run SHALL be released

#### Scenario: Idle capacity borrowing

- **WHEN** a model has demand and another model is idle
- **THEN** the busy model MAY borrow unused active capacity up to its resident ceiling

### Requirement: Safe rollback

The cpu_budget off mode SHALL bypass new calibration and admission policy and preserve legacy scheduling behavior. Turning calibration off SHALL retain conservative planning and process admission. Both modes SHALL be observable and documented.

#### Scenario: Legacy policy

- **WHEN** cpu_budget off is configured
- **THEN** the server SHALL use legacy worker/thread/spinning defaults and report the governor disabled
