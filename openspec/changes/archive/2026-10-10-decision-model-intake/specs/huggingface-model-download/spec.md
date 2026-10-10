# Spec Delta

## ADDED Requirements

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
