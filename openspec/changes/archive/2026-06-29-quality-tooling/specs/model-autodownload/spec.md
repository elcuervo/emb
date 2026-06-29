## ADDED Requirements

### Requirement: Configurable model auto-download

The server SHALL accept a `model_repo` field in the model config to automatically download and export a HuggingFace model.

#### Scenario: model_repo set and model not cached

- **WHEN** a model config has `model_repo` set and the ONNX model path does not exist
- **THEN** the server downloads the model from HuggingFace using `optimum-cli export onnx`, saves it to the configured ONNX path, and loads it

#### Scenario: model_repo set and model already cached

- **WHEN** a model config has `model_repo` set and the ONNX model file already exists
- **THEN** the server loads the cached model without downloading

#### Scenario: model_repo absent

- **WHEN** a model config has no `model_repo` field
- **THEN** the server loads the model from the configured local paths, unchanged from current behavior

#### Scenario: model_repo download failure

- **WHEN** downloading or exporting the model from HuggingFace fails
- **THEN** the server logs the error and exits with non-zero status
