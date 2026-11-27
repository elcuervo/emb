## MODIFIED Requirements

### Requirement: Model configuration from YAML

The server SHALL load model configurations from a YAML file at startup. Only the model name and `onnx` path (or `model_repo`) are required. All other fields have sensible defaults or are auto-detected from the ONNX graph and tokenizer config:

```yaml
listen: ":6379"
models:
  <name>:
    onnx: /path/to/model.onnx          # required (or model_repo)
    model_repo: huggingface/model-id    # optional: auto-download from HF Hub
    tokenizer: /path/to/tokenizer.json  # inferred from onnx dir; optional for image-only models
    dim: 768                            # inferred from ONNX output shape
    max_length: 256                     # inferred from config.json
    pooling: mean                       # inferred from output rank: mean or none
    normalize: true                     # default: true
    output_tensor: last_hidden_state    # inferred from ONNX available outputs
    preload: false                      # default: false (lazy load)
    workers: 0                          # default: 0 (auto-tune by RAM)
```

A model that serves only images — it declares an `image:` block and no tokenizer file exists beside its graph — SHALL load without a tokenizer. Text-only models SHALL still require one, and a command that needs text on an image-only model SHALL fail with an error naming the model and the missing tokenizer rather than a generic load failure. A model that does not declare an `image:` block and has no resolvable tokenizer SHALL fail model loading, as before.

#### Scenario: Minimal config loads correctly

- **WHEN** server starts with a config containing only `onnx` path per model
- **THEN** `dim` is read from ONNX graph, `max_length` from tokenizer config, `pooling` inferred from output rank, `normalize` defaults to `true`, `output_tensor` detected from available ONNX outputs

#### Scenario: Fully specified config still works

- **WHEN** server starts with a config containing all explicit fields
- **THEN** all explicit values are used

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

#### Scenario: Image-only model loads without a tokenizer

- **WHEN** a model declares an `image:` block whose directory carries no `tokenizer.json`
- **THEN** the model loads, serves image commands, and reports its outputs, without a tokenizer being required

#### Scenario: Text on an image-only model fails clearly

- **WHEN** a client sends a text embedding command for an image-only model
- **THEN** the server replies with an error naming the model and the missing tokenizer, and does not attempt inference

#### Scenario: Text model without a tokenizer still fails at load

- **WHEN** a model declares no `image:` block and its tokenizer file cannot be found
- **THEN** model loading fails with an error naming the model and the tokenizer path
