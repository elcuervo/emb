# ctranslate2-backend Specification

## Purpose
Specifies serving BERT-family embeddings through the CTranslate2 backend for layer fusion, variable-length batching without padding, and int8 CPU compute on ARM64/Graviton.

## ADDED Requirements

### Requirement: Backend selection
The server SHALL select an inference backend per the model's `backend` setting.

#### Scenario: auto selects by model format
- **WHEN** a model config sets `backend: auto` and the model directory contains CT2 files (`model.bin` + vocab)
- **THEN** the server SHALL use the CTranslate2 backend
- **WHEN** no CT2 files exist
- **THEN** the server SHALL fall back to the ONNX backend

#### Scenario: explicit onnx
- **WHEN** a model config sets `backend: onnx`
- **THEN** the server SHALL use ONNX Runtime regardless of the model format

### Requirement: Session interface parity
The CTranslate2 backend SHALL satisfy the same `onnx.Session` contract as the ONNX backend.

#### Scenario: Drop-in parity
- **WHEN** the same `EMB` texts are embedded with `backend: onnx` and `backend: ctranslate2`
- **THEN** each backend SHALL produce embeddings through the same pipeline code paths
- **THEN** the RESP responses SHALL differ only within the documented numeric tolerance

### Requirement: Variable-length batches without padding
The CTranslate2 backend SHALL pass variable-length token sequences directly, without padding to a shared batch length.

#### Scenario: Mixed-length run
- **WHEN** a batch contains texts of differing token lengths
- **THEN** the CT2 backend SHALL run them in one call without padding the shorter texts

### Requirement: Composes with batching and async tokenization
While active, batch budget and async tokenization SHALL remain effective.

#### Scenario: Token budget applies
- **WHEN** P1 token-budget batching is enabled with the CT2 backend
- **THEN** the batcher SHALL still bound per-run memory using real token counts

### Requirement: Backend observability
The server SHALL report the active backend.

#### Scenario: EMB.INFO reports backend
- **WHEN** a client calls `EMB.INFO <model>`
- **THEN** the response SHALL include `backend: onnx|ctranslate2`