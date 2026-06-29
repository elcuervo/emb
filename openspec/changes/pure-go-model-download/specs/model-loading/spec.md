## MODIFIED Requirements

### Requirement: Model repository download

The `model_repo` field SHALL download pre-converted ONNX models via the HuggingFace Hub HTTP API. The downstream behavior is unchanged: downloaded files are saved to the configured `onnx` path and loaded by the pipeline.

#### Scenario: model_repo works without Python

- **WHEN** `model_repo` is set in config
- **THEN** the download does NOT require Python, `optimum-cli`, or any shell command
- **THEN** it uses standard Go `net/http` to download model files

#### Scenario: Downloaded files identical to cached

- **WHEN** a model downloaded via the new HTTP path is loaded
- **THEN** the ONNX, tokenizer, and config files are fully compatible with the existing pipeline
