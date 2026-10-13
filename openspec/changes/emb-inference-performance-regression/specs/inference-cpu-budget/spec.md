# Spec Delta

## MODIFIED Requirements

### Requirement: Shared inference admission

Embedding, scripted, image, and calibration ONNX runs SHALL share one fair process-local admission controller. Tokens SHALL be acquired immediately around actual Session.Run and released on every result path. Idle model capacity SHALL be borrowable: a waiter whose width fits the current usage SHALL be admitted even while an earlier, wider waiter is still blocked. Explicit work wider than the budget SHALL run alone with a warning. Admission SHALL be bounded — a caller SHALL supply a deadline, the controller SHALL return a busy or timeout outcome rather than waiting without limit, it SHALL refuse a queue beyond its configured bound, and it SHALL abandon admission for a request whose client has gone away. While the process budget admits more than one run, no single run SHALL hold the whole budget once other work is pending. The controller SHALL report admission wait separately from preprocessing, local pool wait, and run time. Tokens are an admission bound, not an OS CPU limit.

#### Scenario: Concurrent models

- **WHEN** multiple models execute concurrently
- **THEN** their admitted work SHALL share the process budget and preserve fair progress

#### Scenario: Failed run

- **WHEN** an ONNX run returns an error
- **THEN** all admission capacity owned by that run SHALL be released

#### Scenario: Idle capacity borrowing

- **WHEN** a model has demand, another model is idle, and a narrow run waits behind a wider run that cannot be admitted
- **THEN** the narrow run SHALL be admitted into the idle capacity without waiting for the wider run to complete
- **AND** the busy model SHALL NOT be admitted beyond its resident ceiling

#### Scenario: Bounded wait

- **WHEN** the budget stays exhausted beyond the caller's deadline
- **THEN** admission SHALL return a bounded busy/timeout outcome to the caller
- **AND** the caller SHALL NOT be left waiting without a bound

#### Scenario: Bounded queue

- **WHEN** pending admission exceeds the controller's configured queue bound
- **THEN** further requests SHALL be refused with a busy outcome rather than queued
- **AND** the refusal SHALL be counted and observable

#### Scenario: Abandoned request

- **WHEN** the client disconnects or abandons a request that is waiting for admission
- **THEN** the controller SHALL release the pending wait and its accounting
- **AND** the abandoned request SHALL NOT occupy capacity or be run afterwards

#### Scenario: No monopolizing run

- **WHEN** the process budget admits at least two runs and a run of the full budget width is in flight while other work is pending
- **THEN** the layout SHALL admit the pending work rather than serializing it behind that run

#### Scenario: Admission wait reported

- **WHEN** any run waits for admission
- **THEN** the elapsed admission wait SHALL be recorded separately from local pool wait, preprocessing and run time
- **AND** it SHALL be visible in `EMB.INFO`/`EMB.STATS` and the inference metrics

### Requirement: Bounded automatic calibration

Unset embedding session and thread dimensions SHALL be selected by local calibration after model metadata resolution and determinism probing. Calibration SHALL use representative short, typical, and long inputs, separate warmup from repeated trials, check correctness, and obey the process CPU share and conservative model-size memory estimate. It SHALL reject candidates whose p95 exceeds twice the single-worker baseline, choose the highest measured throughput among valid candidates, and prefer wider threads on a tie when the model batches — the trial serves a smaller batch than the coalescing pool — and lower resource use otherwise. A slow in-flight model call may exceed the nominal deadline; temporary sessions SHALL close only after calls return. Calibration SHALL NOT run on the request path and a request SHALL NOT block on it. When the trial ends without a measured winner — deadline truncation, failed baseline, unavailable CPU accounting, or rejected candidates — the layout SHALL be recorded as unresolved with its reason, and an unresolved layout SHALL NOT be presented as a calibrated choice.

#### Scenario: Explicit settings

- **WHEN** a worker count or intra-op width is explicitly configured
- **THEN** calibration SHALL not replace that dimension

#### Scenario: Calibration cannot establish a valid candidate

- **WHEN** a fixture is invalid, candidate execution fails, or CPU accounting is unavailable
- **THEN** the server SHALL use the documented safe fallback and log the reason

#### Scenario: Probe failure

- **WHEN** batch determinism fails
- **THEN** calibration SHALL measure single-row inference and the resulting pool SHALL not retain batch-sized worker defaults

#### Scenario: Calibration is not reachable from a request

- **WHEN** a model is configured for automatic calibration and its layout is not yet decided
- **THEN** calibration SHALL complete before the model is reported ready
- **AND** a request SHALL NOT trigger calibration, SHALL NOT wait on another request's calibration, and SHALL receive an explicit not-ready/busy answer while the layout is undecided

#### Scenario: Truncated trial

- **WHEN** the calibration deadline is reached before the search is exhausted
- **THEN** the outcome SHALL be recorded as unresolved with the deadline reason
- **AND** the smallest measured layout SHALL NOT be reported as the calibrated winner

#### Scenario: Layout is reported

- **WHEN** a model becomes ready
- **THEN** its selected sessions, intra-op width, admission share and calibration source/reason SHALL be reported in the boot log and in `EMB.INFO`
- **AND** a fallback or unresolved layout SHALL be distinguishable from a calibrated one
