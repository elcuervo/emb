## ADDED Requirements

### Requirement: Model configuration from YAML

The server SHALL load model configurations from a YAML file at startup, mapping model names to their ONNX files, tokenizer files, and options.

#### Scenario: Valid config file

- **WHEN** server starts with a valid `config.yaml` containing one or more model entries
- **THEN** each model is loaded: ONNX session created, tokenizer loaded, worker pool initialized with `GOMAXPROCS` workers

#### Scenario: Config file not found

- **WHEN** server starts and the config file does not exist
- **THEN** server logs an error and exits with non-zero status

#### Scenario: Invalid model path

- **WHEN** a model's ONNX file path in config does not exist
- **THEN** server logs which model failed and exits with non-zero status

#### Scenario: Invalid tokenizer path

- **WHEN** a model's tokenizer file path in config does not exist
- **THEN** server logs which model failed and exits with non-zero status

### Requirement: Worker pool auto-tuning

The server SHALL create one worker per CPU core (`GOMAXPROCS` or `runtime.NumCPU`) for each model.

#### Scenario: Worker count matches CPU count

- **WHEN** server starts with GOMAXPROCS=8
- **THEN** each model initializes 8 workers, each with its own `DynamicAdvancedSession` and pre-allocated tensors

#### Scenario: Worker pool serves requests

- **WHEN** multiple embedding requests arrive concurrently for the same model
- **THEN** they are distributed round-robin across the worker pool

### Requirement: Config format specification

The server SHALL accept a YAML config with the following structure:

```yaml
listen: ":6379"
models:
  <name>:
    onnx: /path/to/model.onnx
    tokenizer: /path/to/tokenizer.json
    pooling: mean       # or cls
    normalize: true     # L2 normalize output
    max_length: 64      # truncation length
    dim: 768            # embedding dimension
```

#### Scenario: Config parsed correctly

- **WHEN** server starts with a valid YAML config following the above format
- **THEN** each model's `name`, `onnx`, `tokenizer`, `pooling`, `normalize`, `max_length`, and `dim` are correctly loaded
