# Spec Delta

## MODIFIED Requirements

### Requirement: Concurrent multi-model session tuning

The server SHALL allow tuning ONNX session execution (execution mode and intra-op thread count) per model so two models (e.g. `siglip2` and `e5`) serving concurrently do not unnecessarily contend for CPU, without changing the returned embeddings. When `workers` or `intra_op_threads` are unset, the server SHALL select them by bounded local calibration after the batch determinism probe, obeying the process CPU share and the conservative model-size memory estimate, and SHALL fall back safely when no candidate qualifies. Explicit settings SHALL remain authoritative, and shared process admission SHALL cover embedding, script, and image runs. The selected layout SHALL preserve process concurrency: while the process budget admits more than one run and demand exists, a model with a share of at least two SHALL have at least two concurrently admissible runs.

#### Scenario: Two models served concurrently

- **WHEN** an `EMB.MULTI` embeds the same text through two models
- **THEN** both models SHALL return correct, length-`dim*4` embeddings
- **AND** their ONNX runs SHALL share the process admission budget

#### Scenario: Default configuration

- **WHEN** models are configured without worker or thread overrides
- **THEN** the server SHALL calibrate each embeddable model at load and serve valid embeddings

#### Scenario: Derivation uses the container budget

- **WHEN** `workers` and `intra_op_threads` are unset inside a container whose limits are smaller than the host's
- **THEN** the derived worker and thread counts SHALL be bounded by the container's process plan, not the host's totals

#### Scenario: Probe degradation

- **WHEN** a model fails batch determinism
- **THEN** its layout SHALL serve the single-row path and SHALL not retain workers intended only for batching
- **AND** the resulting concurrency SHALL still admit more than one run when the budget allows it

#### Scenario: Concurrency is preserved

- **WHEN** the process budget admits at least two runs and concurrent plain `EMB` requests are offered to one model
- **THEN** more than one run SHALL be concurrently admissible
- **AND** the reported sessions and intra-op width SHALL account for the whole share

#### Scenario: A reduced layout is visible

- **WHEN** the derived layout reduces a model to a single session of one thread per worker
- **THEN** that layout SHALL be distinguishable from a calibrated one in the boot log and in `EMB.INFO`

## ADDED Requirements

### Requirement: Bounded plain embedding latency under overload

The server SHALL keep plain `EMB` latency bounded under overload: excess work SHALL be refused with a busy outcome or delayed only within a bounded admission queue, and latency SHALL NOT grow with the number of waiting requests. Servicing SHALL be work-conserving, work whose client has gone away SHALL NOT run to completion, and health commands SHALL stay answerable while embeddings are saturated.

#### Scenario: Burst stays bounded

- **GIVEN** a served model with a decided layout and a process budget of at least two runs
- **WHEN** a burst of concurrent plain `EMB` requests arrives, well above the admissible concurrency
- **THEN** the requests SHALL either complete inside the stated latency bound or be refused with a busy outcome
- **AND** the tail latency SHALL NOT grow without bound with the burst size

#### Scenario: Cold cache burst

- **WHEN** the same burst arrives with a cold reply cache and a cold model
- **THEN** the model's layout decision SHALL complete before the model serves, and no request SHALL wait on another request's initialization
- **AND** the latency bound SHALL hold once the model is ready

#### Scenario: Co-tenant heavy work

- **WHEN** a long or wide run on another model or path is in flight
- **THEN** a short plain `EMB` request SHALL make progress using idle capacity rather than waiting for the long run to finish
- **AND** the short request's admission wait SHALL be recorded

#### Scenario: Abandoned work drains

- **WHEN** clients time out and disconnect while requests are queued or in flight
- **THEN** the outstanding queue SHALL drain without a restart
- **AND** subsequent requests SHALL be served inside the latency bound

#### Scenario: Health stays answerable

- **WHEN** embedding admission is saturated or refusing work
- **THEN** `PING`, `EMB.READY` and introspection SHALL still answer

### Requirement: Regression gate for the plain embedding path

The repository SHALL provide a regression gate for plain `EMB`, independent of the scripted path: deterministic model-free tests of admission bounding and layout concurrency, an in-process gate that drives concurrent plain `EMB` against an in-repo fixture inside the existing CGo test job without a network download, and a same-host differential between two revisions on one qualified host.

#### Scenario: The frozen pair separates

- **WHEN** the differential runs the last known-good revision and the failing revision on one qualified host with identical configuration, model and load
- **THEN** the failing revision SHALL exceed the plain-`EMB` latency bound or fail the admission/queue assertions
- **AND** the known-good revision SHALL pass the same run

#### Scenario: Signals are recorded

- **WHEN** the differential completes a run
- **THEN** its evidence SHALL record latency percentiles, throughput, errors/timeouts, admission wait, queue depth and the effective layout, with the outcome of every assertion it made

#### Scenario: Incomparable runs are refused

- **WHEN** two runs with different host fingerprints are compared
- **THEN** the tooling SHALL refuse and print both fingerprints

#### Scenario: Unmet precondition is not a pass

- **WHEN** a required phase cannot be exercised on the chosen host, or the run cannot complete its matrix
- **THEN** the affected assertion SHALL be recorded as skipped or invalid with the host property named
- **AND** the run SHALL NOT be reported as a pass

#### Scenario: The scripted gate is not a substitute

- **WHEN** only the scripted-path gate runs
- **THEN** a plain embedding admission regression SHALL NOT be reported as covered

### Requirement: The gate is self-verifying and routine

The gate SHALL be verified to fail on the known regressions — an admission wait without a deadline, an unbounded queue, a layout pinned to a single single-threaded run, calibration reachable from a request, and reply-cache hits counted as traffic — and each failure SHALL name the assertion that detected it. The deterministic and in-process tiers SHALL run when the inference, registry, pipeline or server packages change; the differential SHALL run for a release.

#### Scenario: Known regressions are caught

- **WHEN** any of the listed regressions is applied to the tree
- **THEN** the gate SHALL fail
- **AND** it SHALL name the assertion that detected the regression

#### Scenario: Unmodified tree passes

- **WHEN** the tree is unmodified
- **THEN** the gate SHALL pass, reporting only the documented host-dependent skips

#### Scenario: The gate is routine

- **WHEN** a change modifies the inference, registry, pipeline or server packages
- **THEN** the deterministic and in-process tiers SHALL run in the existing test jobs without a network download
- **AND** the differential SHALL run on the qualified host for a release, not against a baseline from a different host
