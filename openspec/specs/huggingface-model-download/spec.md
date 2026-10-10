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

### Requirement: Model download from a repository subfolder

A model config SHALL be able to name a subfolder within `model_repo`, so a
repository that publishes several checkpoints under separate folders can be mounted
one at a time. When a subfolder is set, the downloader SHALL resolve the ONNX file
and its supporting files under that subfolder and write them into the configured
model directory under their conventional names, and it SHALL fail with a clear error
when the subfolder holds no ONNX file.

#### Scenario: A checkpoint published under a folder

- **WHEN** a model config sets `model_repo: codenamev/laya-onnx` and `model_subfolder: multilingual`
- **THEN** the downloader fetches `multilingual/model.onnx` and the supporting files under `multilingual/`, and writes them into the model's directory

#### Scenario: A subfolder with no ONNX file

- **WHEN** the named subfolder holds no `.onnx` file
- **THEN** the download fails with an error naming the repository and the subfolder

#### Scenario: No subfolder keeps the current behaviour

- **WHEN** a model config sets `model_repo` without a subfolder
- **THEN** the downloader resolves the ONNX from the repository root as before

### Requirement: ONNX external-data sidecars are downloaded with the graph

When a repository ships the selected ONNX graph without its weights — the graph names an
external-data file beside it — the downloader SHALL transfer those sidecars into the same
directory as the graph, under their original base names, before the model is loaded. A
transfer that fails SHALL fail the download with an error naming the repository and the
missing sidecar.

#### Scenario: Split export downloads whole

- **WHEN** a model config names a repository whose selected graph is `onnx/model.onnx` and the repository also holds `onnx/model.onnx_data`
- **THEN** both files SHALL be downloaded and published into the model directory, so the graph loads

#### Scenario: Self-contained graph is unaffected

- **WHEN** the selected graph holds all of its weights and the repository ships no sidecar for it
- **THEN** the download performs exactly as before, with no extra transfer

#### Scenario: Sidecar transfer fails

- **WHEN** a sidecar exists in the repository but its transfer fails
- **THEN** the download SHALL fail with an error naming the sidecar, and the model SHALL NOT be reported as ready

#### Scenario: A subfolder keeps its sidecars together

- **WHEN** a model config sets `model_subfolder` and the graph under that subfolder has a sidecar in the same subfolder
- **THEN** the sidecar SHALL be resolved under that subfolder and written beside the graph under its base name

### Requirement: Quantized preference does not change a mount that ships no quantized artifact

A repository that ships neither a `model_quantized.onnx` member nor an int8-named member
SHALL resolve to the same graph it resolved to before quantized resolution learned the int8
names, so the artifact an existing mount loads does not move underneath it.

#### Scenario: A repository with no quantized member is unchanged

- **WHEN** a repository publishes only `model.onnx` per subfolder, as `codenamev/laya-onnx` does
- **THEN** quantized resolution SHALL find no quantized member and SHALL NOT substitute one

#### Scenario: An existing subfolder mount keeps its artifact

- **WHEN** a model config sets `model_repo: codenamev/laya-onnx` and `model_subfolder: typed-decisions` with `quantize` unset
- **THEN** the resolved graph SHALL be `typed-decisions/model.onnx`, as before this change
