# Spec Delta

## ADDED Requirements

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
