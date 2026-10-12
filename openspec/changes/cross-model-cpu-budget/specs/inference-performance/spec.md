## MODIFIED Requirements

### Requirement: Safe automatic embedding layout

Unset embedding worker and intra-op settings SHALL be selected by bounded local calibration after the batch determinism probe. Calibration SHALL preserve output correctness, obey the process CPU share and conservative model-size memory estimate, and use a safe fallback if no candidate qualifies. Explicit settings SHALL remain authoritative. Shared process admission SHALL cover embedding, script, and image runs.

#### Scenario: Default configuration

- **WHEN** models are configured without worker or thread overrides
- **THEN** the server SHALL calibrate each embeddable model at load and serve valid embeddings

#### Scenario: Probe degradation

- **WHEN** a model fails batch determinism
- **THEN** its layout SHALL serve the single-row path and SHALL not retain workers intended only for batching

#### Scenario: Concurrent models

- **WHEN** models receive concurrent requests
- **THEN** their ONNX runs SHALL share the process admission budget

### Requirement: Local validation

The repository SHALL provide deterministic model-free tests for calibration candidate bounds and admission behavior. The full Go suite, lint/vet, and a local ONNX server smoke SHALL run inside `nix develop`.

#### Scenario: Local checks

- **WHEN** a developer runs the documented checks
- **THEN** they SHALL run without Datadog credentials or production services
