## ADDED Requirements

### Requirement: Model download from HuggingFace Hub via HTTP

The server SHALL download models from HuggingFace Hub using pure Go HTTP calls, without shelling out to `optimum-cli` or requiring Python.

#### Scenario: Download pre-converted ONNX model

- **WHEN** a model config has `model_repo` set and the ONNX path doesn't exist locally
- **THEN** the server queries `https://huggingface.co/api/models/{repo}` to find ONNX files
- **THEN** the server downloads `model.onnx`, `tokenizer.json`, `config.json`, and supporting files via HTTPS
- **THEN** the downloaded model is loaded normally

#### Scenario: No ONNX files in repo

- **WHEN** the specified repo has no `.onnx` files
- **THEN** the server fails with a clear error message: no ONNX files found, suggest using `optimum-cli` manually

#### Scenario: Network failure during download

- **WHEN** the download fails (network error, timeouts)
- **THEN** the server logs the error and exits with non-zero status (unchanged behavior)
