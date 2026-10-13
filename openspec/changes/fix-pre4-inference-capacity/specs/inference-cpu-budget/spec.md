# Spec Delta

## MODIFIED Requirements

### Requirement: Bounded automatic calibration

Unset embedding session and thread dimensions SHALL be selected by local calibration after model metadata resolution and determinism probing. Calibration SHALL use representative short, typical, and long inputs, separate warmup from repeated trials, check correctness, and obey the process CPU ceiling and process-wide memory headroom rather than a permanent per-model share ceiling. It SHALL reject candidates whose p95 exceeds twice the single-worker baseline, choose the highest measured throughput among valid candidates, and prefer wider threads on a tie when the model batches and lower resource use otherwise. The total trial budget SHALL account for session construction, warmup, execution, and cleanup. A slow in-flight native call may exceed the nominal deadline; temporary sessions SHALL close only after calls return.

#### Scenario: Explicit settings
- **WHEN** a worker count or intra-op width is explicitly configured
- **THEN** calibration SHALL not replace that dimension

#### Scenario: Calibration cannot establish a valid candidate
- **WHEN** a fixture is invalid, candidate execution fails, or CPU accounting is unavailable
- **THEN** the server SHALL use the documented safe fallback and log the reason

#### Scenario: Probe failure
- **WHEN** batch determinism fails
- **THEN** calibration SHALL measure single-row inference and the resulting pool SHALL not retain batch-sized worker defaults

#### Scenario: Slow candidate construction
- **WHEN** session creation and warmup dominate a candidate's cost
- **THEN** subsequent trial admission SHALL account for that cost rather than only its timed inference section

#### Scenario: Idle peer does not impose a permanent ceiling
- **WHEN** another configured model is idle and memory permits a larger validated resident layout
- **THEN** the active model's candidate ceiling SHALL NOT be reduced solely to its static weighted share

### Requirement: Shared inference admission

Embedding, scripted, image, and calibration ONNX runs SHALL share one fair process-local admission controller. Tokens SHALL be acquired immediately around actual Session.Run and released on every result path. Idle model capacity SHALL be borrowable. Model weights SHALL govern fair turns under contention, not permanent idle reservations. FIFO order within each model and bounded progress for wide runs SHALL be preserved. Explicit work wider than the budget SHALL run alone with a warning. The controller SHALL report admission wait separately from preprocessing, local pool wait, and run time. Tokens are an admission bound, not an OS CPU limit.

#### Scenario: Concurrent models
- **WHEN** multiple models execute concurrently
- **THEN** their admitted work SHALL share the process budget and preserve fair progress

#### Scenario: Failed run
- **WHEN** an ONNX run returns an error
- **THEN** all admission capacity owned by that run SHALL be released

#### Scenario: Idle capacity borrowing
- **WHEN** a model has demand and another model is idle
- **THEN** unused active capacity SHALL be available to the busy model up to its validated resident ceiling

#### Scenario: Wide request under sustained narrow traffic
- **WHEN** a wide request waits while other models continuously submit narrower requests
- **THEN** the wide request SHALL receive a bounded scheduling turn rather than be bypassed indefinitely

#### Scenario: Shared session accounting
- **WHEN** one or more callers execute against a shared native session
- **THEN** admission SHALL account for its active shared worker pool and concurrent caller execution without assuming every configured caller is active

## ADDED Requirements

### Requirement: Consistent resource resolution

Startup planning and saturation accounting SHALL use a common resource-resolution contract retaining fractional CPU bandwidth and distinguishing it from affinity, scheduler parallelism, and integer admission capacity. Applicable visible cgroup ancestor limits and controller mount roots SHALL be honored. Missing or unreadable limits SHALL carry explicit fallback provenance.

#### Scenario: Remapped or combined controller mount
- **WHEN** a CPU controller uses a combined v1 mount or a remapped cgroup root
- **THEN** the effective allowance SHALL reflect the process's applicable limit rather than an assumed filesystem location

#### Scenario: Unlimited child with constrained parent
- **WHEN** a child has unlimited CPU bandwidth but a visible ancestor is limited
- **THEN** the tightest applicable ancestor bandwidth SHALL constrain the resolved allowance

#### Scenario: Affinity and scheduler bounds
- **WHEN** affinity or an explicit lower scheduler setting is tighter than quota
- **THEN** the planning ceiling SHALL reflect the applicable tighter bound while retaining the distinct source values

### Requirement: Process-wide resident memory admission

Automatic resident expansion and temporary calibration sessions SHALL share a process-wide memory headroom calculation accounting for already resident resources, remaining configured model reservations, cache allowance, and external model data. Concurrent model loads SHALL NOT each reserve the same free headroom. A memory-limited smaller layout SHALL report its limiting reason.

#### Scenario: Concurrent lazy models
- **WHEN** two lazy models request additional sessions against limited remaining memory
- **THEN** their combined reservations SHALL remain within the same process headroom bound

#### Scenario: Larger candidate cannot fit
- **WHEN** a larger layout cannot fit but a valid smaller layout can
- **THEN** the smaller layout SHALL remain usable and its memory-limited reason SHALL be observable

### Requirement: Bounded cancellable admission waiting

The boot-time `cpu_admission_timeout` duration SHALL bound each CPU-gate wait when positive; omitted or zero SHALL preserve unlimited waiting. Negative or invalid values SHALL fail validation. Expiry SHALL return `ERR busy: CPU admission timeout` through the existing command error contract. Cancellation or expiry SHALL remove waiting work without consuming tokens or starting its native call.

#### Scenario: Wait expires
- **WHEN** a queued run reaches its configured admission timeout
- **THEN** it SHALL leave the queue, report the timeout, and never execute that run

#### Scenario: Grant races cancellation
- **WHEN** cancellation races a grant
- **THEN** exactly one outcome SHALL own the grant and all acquired tokens SHALL be released exactly once

#### Scenario: Native work already started
- **WHEN** a native call has already acquired admission
- **THEN** the admission timeout SHALL NOT destroy its resources or claim to interrupt its execution
