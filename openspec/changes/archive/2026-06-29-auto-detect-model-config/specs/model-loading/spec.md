## MODIFIED Requirements

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
