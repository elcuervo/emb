## ADDED Requirements

### Requirement: Model configuration from YAML

The server SHALL load model configurations from a YAML file at startup. Only the model name and `onnx` path (or `model_repo`) are required. All other fields have sensible defaults or are auto-detected from the ONNX graph and tokenizer config:

```yaml
listen: ":6379"
models:
  <name>:
    onnx: /path/to/model.onnx          # required (or model_repo)
    model_repo: huggingface/model-id    # optional: auto-download from HF Hub
    tokenizer: /path/to/tokenizer.json  # inferred from onnx dir if absent
    dim: 768                            # inferred from ONNX output shape
    max_length: 256                     # inferred from config.json
    pooling: mean                       # inferred from output rank: mean or none
    normalize: true                     # default: true
    output_tensor: last_hidden_state    # inferred from ONNX available outputs
    preload: false                      # default: false (lazy load)
    workers: 0                          # default: 0 (auto-tune by RAM)
```

#### Scenario: Minimal config loads correctly

- **WHEN** server starts with a config containing only `onnx` path per model
- **THEN** `dim` is read from ONNX graph, `max_length` from tokenizer config, `pooling` inferred from output rank, `normalize` defaults to `true`, `output_tensor` detected from available ONNX outputs

#### Scenario: Fully specified config still works

- **WHEN** server starts with a config containing all explicit fields
- **THEN** all explicit values are used (backward compatible)

#### Scenario: Config with preload and workers parsed correctly

- **WHEN** server starts with config containing `preload: true` and `workers: 4`
- **THEN** the model is loaded at startup with exactly 4 workers

#### Scenario: Config without preload defaults to lazy

- **WHEN** server starts with config that omits `preload`
- **THEN** the model is registered but not loaded until the first EMB request

#### Scenario: Model loads with tokenizer path

- **WHEN** a config specifies `tokenizer: /path/to/tokenizer.json`
- **THEN** the tokenizer is loaded via `daulet/tokenizers` instead of the hand-rolled implementation
- **THEN** behavior is identical for all previously supported WordPiece and BPE models

### Requirement: Model repository download

The `model_repo` field SHALL download pre-converted ONNX models via the HuggingFace Hub HTTP API. The downstream behavior is unchanged: downloaded files are saved to the configured `onnx` path and loaded by the pipeline.

#### Scenario: model_repo works without Python

- **WHEN** `model_repo` is set in config
- **THEN** the download does NOT require Python, `optimum-cli`, or any shell command
- **THEN** it uses standard Go `net/http` to download model files

#### Scenario: Downloaded files identical to cached

- **WHEN** a model downloaded via the new HTTP path is loaded
- **THEN** the ONNX, tokenizer, and config files are fully compatible with the existing pipeline
