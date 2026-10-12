## MODIFIED Requirements

### Requirement: Concurrent multi-model session tuning

The server SHALL allow tuning ONNX session execution (execution mode and intra-op thread count) per model so two models (e.g. `siglip2` and `e5`) serving concurrently do not unnecessarily contend for CPU, without changing the returned embeddings. When `workers` or `intra_op_threads` are unset, the server SHALL select them by bounded local calibration after the batch determinism probe, obeying the process CPU share and the conservative model-size memory estimate, and SHALL fall back safely when no candidate qualifies. Explicit settings SHALL remain authoritative, and shared process admission SHALL cover embedding, script, and image runs.

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

## ADDED Requirements

### Requirement: Local validation

The repository SHALL provide deterministic model-free tests for calibration candidate bounds and admission behavior. The full Go suite, lint/vet, and a local ONNX server smoke SHALL run inside `nix develop`.

#### Scenario: Local checks

- **WHEN** a developer runs the documented checks
- **THEN** they SHALL run without Datadog credentials or production services
