## MODIFIED Requirements

### Requirement: Config format specification

The server SHALL accept a YAML config where only the model name and `onnx` path are required. All other fields are optional with auto-detected defaults:

```yaml
listen: ":6379"
models:
  <name>:
    onnx: /path/to/model.onnx
    # All below are optional:
    tokenizer: /path/to/tokenizer.json   # inferred from onnx dir if absent
    pooling: mean                         # default: mean
    normalize: true                       # default: true
    max_length: 256                       # inferred from tokenizer/config.json
    dim: 768                              # inferred from ONNX graph
```

#### Scenario: Minimal config loads correctly

- **WHEN** server starts with a config containing only `onnx` path per model
- **THEN** `dim` is read from ONNX graph, `max_length` from tokenizer config, `pooling` defaults to `mean`, `normalize` defaults to `true`

#### Scenario: Fully specified config still works

- **WHEN** server starts with a config containing all explicit fields
- **THEN** all explicit values are used (backward compatible)
