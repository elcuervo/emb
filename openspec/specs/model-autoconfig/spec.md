# model-autoconfig Specification

## Purpose
Specifies auto-detection of model settings from the ONNX graph and tokenizer config: embedding dim, max sequence length, pooling/normalization, and output tensor — with explicit config overriding.

## Requirements
### Requirement: Auto-detect embedding dimension from ONNX graph

The server SHALL read the ONNX model's output tensor shape to determine the embedding dimension.

#### Scenario: dim detected from last_hidden_state

- **WHEN** a model is loaded with only `onnx` path (no `dim` in config)
- **THEN** the server inspects the ONNX graph's `last_hidden_state` output shape and extracts the last dimension as `dim`

#### Scenario: explicit dim overrides auto-detect

- **WHEN** `dim` is set in the model config
- **THEN** the configured value is used regardless of the ONNX graph shape

### Requirement: Auto-detect max sequence length

The server SHALL determine `max_length` from the tokenizer configuration or model config.json.

#### Scenario: max_length from tokenizer config

- **WHEN** a model is loaded with only `onnx` path (no `max_length` in config)
- **THEN** the server reads the tokenizer's `max_length` or the model's `max_position_embeddings` from `config.json` in the same directory

#### Scenario: explicit max_length overrides auto-detect

- **WHEN** `max_length` is set in the model config
- **THEN** the configured value is used

### Requirement: Default pooling and normalization

The server SHALL default to `mean` pooling and `normalize: true` when not specified.

#### Scenario: pooling and normalize not in config

- **WHEN** a model config omits `pooling` and `normalize`
- **THEN** `pooling` defaults to `mean` and `normalize` defaults to `true`

#### Scenario: explicit pooling or normalize overrides default

- **WHEN** `pooling` or `normalize` are set in the config
- **THEN** the configured values are used

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
