## MODIFIED Requirements

### Requirement: Model configuration from YAML

The server SHALL load model configurations from a YAML file at startup, mapping model names to their ONNX files, tokenizer files, and options. Models SHALL also accept an optional `model_repo` field for automatic download from HuggingFace.

#### Scenario: Load model with model_repo

- **WHEN** a model config includes `model_repo` and either no ONNX path or the ONNX path doesn't exist
- **THEN** the server downloads the model from HuggingFace using `optimum-cli export onnx` to the model path, then loads it

#### Scenario: Unknown model (unchanged)

- **WHEN** config references a model with neither `model_repo` nor valid local paths
- **THEN** server logs which model failed and exits with non-zero status

### Requirement: Config format specification

The server SHALL accept a YAML config with an optional `model_repo` field:

```yaml
listen: ":6379"
models:
  <name>:
    model_repo: huggingface/model-id  # optional
    onnx: /path/to/model.onnx          # optional if model_repo is set
    tokenizer: /path/to/tokenizer.json  # optional if model_repo is set
    pooling: mean
    normalize: true
    max_length: 256
    dim: 384
```

#### Scenario: Config with model_repo parsed correctly

- **WHEN** server starts with a valid YAML config containing `model_repo`
- **THEN** the `model_repo` field is correctly parsed and stored in the config struct
