## ADDED Requirements

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
