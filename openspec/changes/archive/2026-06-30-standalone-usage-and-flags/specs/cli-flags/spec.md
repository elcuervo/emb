## ADDED Requirements

### Requirement: Flag-based configuration (no YAML required)

The `emb` binary SHALL accept a `-listen` flag to set the server address (default `:6379`).

When `-config` is not provided, the binary SHALL configure itself from CLI flags. At least `-model-onnx` or `-model-repo` SHALL be required to define a model.

When `-config` IS provided, the binary SHALL load the YAML as before. Server-level flags (e.g., `-listen`) MAY override YAML values.

#### Scenario: Run with model repo flag

- **WHEN** `emb -model-repo Xenova/all-MiniLM-L6-v2` is run
- **THEN** the server starts listening on `:6379` and registers a model named `"model"` configured from the `Xenova/all-MiniLM-L6-v2` repo

#### Scenario: Run with explicit ONNX path

- **WHEN** `emb -model-onnx ./model.onnx -model-tokenizer ./tokenizer.json` is run
- **THEN** the server starts with a model configured from the given ONNX and tokenizer files

#### Scenario: Config file still works

- **WHEN** `emb -config prod.yaml` is run
- **THEN** the existing YAML loading behavior is preserved

### Requirement: Model config flags

The binary SHALL expose these model-level flags: `-model`, `-model-onnx`, `-model-repo`, `-model-tokenizer`, `-pooling`, `-normalize`, `-dim`, `-max-length`, `-output-tensor`, `-pad-output`, `-workers`, `-intra-op-threads`, `-inter-op-threads`.

The `-model` flag SHALL set the model key name. When no `-model` flag is given, the model key defaults to `"model"`.

When multiple models are specified via repeated `-model` flags, each `-model` resets the model context. All model-level flags after a `-model` apply to that model until the next `-model` is encountered.

#### Scenario: Full model config via flags

- **WHEN** `emb -model mymodel -model-onnx ./m.onnx -model-tokenizer ./tok.json -pooling mean -normalize -dim 384 -max-length 128 -output-tensor last_hidden_state -pad-output -workers 4` is run
- **THEN** the model `"mymodel"` is registered with all specified config values

#### Scenario: Two models via repeated -model

- **WHEN** `emb -model minilm -model-onnx ./minilm.onnx -model bge -model-repo Xenova/bge-small-en-v1.5` is run
- **THEN** two models are registered: `"minilm"` (from the local ONNX file) and `"bge"` (auto-downloaded from the HuggingFace repo)

### Requirement: Version flag

The binary SHALL accept a `-version` flag that prints the build version and exits with code 0. When no version is embedded at build time, it SHALL print `"dev"`.

#### Scenario: Print version

- **WHEN** `emb -version` is run
- **THEN** the output contains the version string and the process exits with code 0
