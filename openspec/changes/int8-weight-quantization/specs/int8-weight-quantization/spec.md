# int8-weight-quantization Specification

## Purpose
Specifies serving int8-quantized ONNX weights for reduced latency and memory on ARM64/Graviton CPU, using pre-quantized model artifacts rather than runtime quantization.

## ADDED Requirements

### Requirement: Quantization selection
The server SHALL select the inference weights per the model's `quantize` setting.

#### Scenario: auto prefers quantized when present
- **WHEN** a model config sets `quantize: auto` and the model directory ships `model_quantized.onnx` (or `onnx/quantized/model.onnx`)
- **THEN** the server SHALL load the quantized file
- **THEN** no quantized file exists → the server SHALL fall back to fp32 and log a warning

#### Scenario: on requires quantized
- **WHEN** a model config sets `quantize: on` and no quantized file exists
- **THEN** the server SHALL fail model load with a clear error

#### Scenario: off always fp32
- **WHEN** a model config sets `quantize: off`
- **THEN** the server SHALL load the fp32 weights regardless of available quantized files

### Requirement: Quantization-aware download
When downloading a model from HuggingFace with quantization enabled, the downloader SHALL resolve the quantized artifact.

#### Scenario: Quantized download
- **WHEN** `quantize` is enabled and downloading `Xenova/all-MiniLM-L6-v2`
- **THEN** the downloader SHALL prefer `model_quantized.onnx` / `onnx/quantized/model.onnx` over `model.onnx`
- **THEN** the tokenizer and config SHALL be downloaded as usual

### Requirement: Quantization observability
The server SHALL expose the loaded quantization and model size through the stats interface.

#### Scenario: EMB.INFO reports quantization
- **WHEN** a client calls `EMB.INFO <model>`
- **THEN** the response SHALL include `quantization: int8|fp32` and the on-disk model size

### Requirement: Quantized output quality
The server SHALL keep quantized-output quality within tolerance of fp32.

#### Scenario: Cosine tolerance
- **WHEN** the same corpus is embedded with int8 and fp32 weights
- **THEN** per-pair cosine similarity SHALL be ≥ 0.99 over the validation corpus