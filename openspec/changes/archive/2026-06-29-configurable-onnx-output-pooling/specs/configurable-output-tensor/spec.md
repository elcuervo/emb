## ADDED Requirements

### Requirement: Model config accepts output_tensor field
The system SHALL allow each model to declare which ONNX output tensor to use via an `output_tensor` field in YAML config. When unset, the system SHALL default to `"last_hidden_state"`.

#### Scenario: Config with explicit output_tensor
- **WHEN** a model config sets `output_tensor: pooler_output`
- **THEN** the ONNX session SHALL request `pooler_output` as its output tensor name

#### Scenario: Config without output_tensor
- **WHEN** a model config omits `output_tensor`
- **THEN** the ONNX session SHALL request `"last_hidden_state"` (unchanged behavior)

### Requirement: InferDim handles 2D output tensors
The system SHALL correctly infer embedding dimension from ONNX models whose output tensor is 2D (batch × dim), in addition to the existing 3D (batch × seq × dim) case.

#### Scenario: 2D output tensor dim inference
- **WHEN** `InferDim` is called on a model with a 2D output (e.g., `pooler_output` shape [batch, 768])
- **THEN** it SHALL return 768

#### Scenario: 3D output tensor dim inference unchanged
- **WHEN** `InferDim` is called on a model with a 3D output (shape [batch, seq, dim])
- **THEN** it SHALL return dim (last dimension) as before

### Requirement: Invalid output tensor name fails at load time
The system SHALL fail model loading with a descriptive error when the configured `output_tensor` does not exist in the ONNX graph.

#### Scenario: Unknown output tensor
- **WHEN** a model config sets `output_tensor: nonexistent_output`
- **THEN** model loading SHALL fail with an error naming the unrecognized tensor
