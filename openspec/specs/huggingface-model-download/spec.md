# huggingface-model-download Specification

## Purpose
Specifies downloading models from HuggingFace Hub over HTTP (pre-converted ONNX + tokenizer.json) without requiring a local Python install.

## Requirements
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

### Requirement: Atomic download publication
The downloader SHALL write a new artifact to a unique temporary sibling file and publish its final path only after copying and closing succeed. Any failed transfer or publication SHALL return an error and clean up its temporary file without publishing partial bytes.

#### Scenario: Interrupted transfer followed by retry
- **WHEN** an HTTP body fails after some bytes have been written
- **THEN** that attempt SHALL return an error without creating a final cache entry
- **AND** a later retry SHALL perform a new transfer and return the complete artifact

#### Scenario: Close or publication failure
- **WHEN** closing the downloaded file or publishing it to the final path fails
- **THEN** the downloader SHALL return an error and remove its temporary file

#### Scenario: Completed artifact already exists
- **WHEN** a completed local artifact exists at the requested final path
- **THEN** existing reuse behavior SHALL be preserved
