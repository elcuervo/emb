## ADDED Requirements

### Requirement: Output tensor auto-detected from ONNX graph

The server SHALL auto-detect the output tensor name from the ONNX model's available outputs when not explicitly configured.

#### Scenario: rank-2 output preferred over rank-3

- **WHEN** a model has both a rank-2 output (e.g., `pooler_output`) and a rank-3 output (e.g., `last_hidden_state`)
- **AND** `output_tensor` is not set in the config
- **THEN** the server selects the rank-2 output

#### Scenario: single rank-3 output selected

- **WHEN** a model has only one rank-3 output (e.g., `last_hidden_state`)
- **AND** `output_tensor` is not set in the config
- **THEN** the server selects that rank-3 output

#### Scenario: explicit output_tensor wins

- **WHEN** `output_tensor` is set in the model config
- **THEN** the configured value is used regardless of available outputs

### Requirement: Pooling strategy inferred from output rank

The server SHALL infer the pooling strategy from the selected output tensor's rank: rank-2 → `none`, rank-3 → `mean`.

#### Scenario: rank-2 output sets pooling to none

- **WHEN** the selected output tensor has rank 2 (shape `(batch, dim)`)
- **AND** `pooling` is not set in the config
- **THEN** `pooling` is set to `none` (no mean pooling applied)

#### Scenario: rank-3 output sets pooling to mean

- **WHEN** the selected output tensor has rank 3 (shape `(batch, seq_len, dim)`)
- **AND** `pooling` is not set in the config
- **THEN** `pooling` is set to `mean` (mean pool across sequence length)

#### Scenario: explicit pooling wins

- **WHEN** `pooling` is set in the model config
- **THEN** the configured value is used regardless of output rank

### Requirement: Normalization defaults to true

The server SHALL default `normalize` to `true` when not explicitly configured, as this is the correct setting for cosine similarity workloads.

#### Scenario: normalize defaults to true

- **WHEN** `normalize` is not set in the model config
- **THEN** `normalize` defaults to `true`

#### Scenario: explicit normalize wins

- **WHEN** `normalize` is set in the model config
- **THEN** the configured value is used

### Requirement: Auto-detection logged at registration

The server SHALL log the auto-detected `output_tensor`, `pooling`, and `normalize` values at model registration time so users can verify the choices.

#### Scenario: auto-detection logged

- **WHEN** a model is registered without explicit `output_tensor`, `pooling`, or `normalize`
- **THEN** the server logs the detected values for each model
