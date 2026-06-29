## ADDED Requirements

### Requirement: pooling none strategy skips mean pooling
The system SHALL support `pooling: none` as a strategy for models that return pre-pooled 2D output tensors. With this strategy the system SHALL NOT apply mean pooling; it SHALL treat each row of the output as the final embedding vector directly.

#### Scenario: Pre-pooled model embedding
- **WHEN** a model is configured with `pooling: none` and a 2D output tensor
- **THEN** each row of the ONNX output (shape [batch, dim]) SHALL be used directly as the embedding

#### Scenario: pooling none with normalize true
- **WHEN** a model is configured with `pooling: none` and `normalize: true`
- **THEN** each embedding vector SHALL be L2-normalized after extraction

#### Scenario: pooling none with normalize false
- **WHEN** a model is configured with `pooling: none` and `normalize: false`
- **THEN** each embedding vector SHALL be returned as-is without normalization

### Requirement: Existing mean pooling behavior unchanged
The system SHALL preserve all existing behavior for models using `pooling: mean` (or no pooling config). No regressions in output or performance.

#### Scenario: Default pooling unchanged
- **WHEN** a model config omits `pooling` or sets `pooling: mean`
- **THEN** the system SHALL apply mean pooling over the sequence dimension, identical to current behavior

### Requirement: Pooling strategy mismatch detected at load time
The system SHALL validate that the ONNX output tensor rank matches the configured pooling strategy and fail with a descriptive error on mismatch.

#### Scenario: none pooling with 3D tensor
- **WHEN** `pooling: none` is configured but the output tensor is 3D (batch × seq × dim)
- **THEN** model loading SHALL fail with an error indicating rank mismatch

#### Scenario: mean pooling with 2D tensor
- **WHEN** `pooling: mean` is configured (or defaulted) but the output tensor is 2D
- **THEN** model loading SHALL fail with an error indicating rank mismatch
