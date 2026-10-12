# Spec Delta

## MODIFIED Requirements

### Requirement: Quantization selection
The server SHALL select the inference weights per the model's `quantize` setting.

#### Scenario: auto prefers quantized when present
- **WHEN** a model config sets `quantize: auto` and the model directory ships `model_quantized.onnx` (or `onnx/quantized/model.onnx`), or an int8-named artifact `model_int8.onnx` (or `onnx/model_int8.onnx`)
- **THEN** the server SHALL load the quantized file
- **THEN** no quantized file exists → the server SHALL fall back to fp32 and log a warning

#### Scenario: An int8-named artifact outranks a sibling fp32 file
- **WHEN** a model directory holds both `model_fp32.onnx` and `model_int8.onnx` and the model config sets `quantize: auto`
- **THEN** the server SHALL load `model_int8.onnx`, even though `model_fp32.onnx` sorts first by name

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

#### Scenario: An int8-named artifact is resolved
- **WHEN** `quantize` is enabled and the repository ships `model_int8.onnx` (or `onnx/model_int8.onnx`) and no `model_quantized.onnx`
- **THEN** the downloader SHALL transfer the int8-named artifact
- **THEN** it SHALL NOT transfer a sibling `model_fp32.onnx` instead

#### Scenario: An int8-named artifact is labelled int8
- **WHEN** the resolved weight file's base name contains `int8`
- **THEN** the model's reported quantization SHALL be `int8`

## ADDED Requirements

### Requirement: Decision-model answer parity under quantization

A quantized decision graph SHALL answer a decision request the same way its fp32
counterpart does, within tolerance: over a fixed question set, the winning option SHALL match
and every reported probability SHALL differ by no more than 0.05.

#### Scenario: Int8 and fp32 of one checkpoint agree
- **WHEN** the same checkpoint's int8 and fp32 artifacts answer a fixed question set
- **THEN** every answer's winning option SHALL match, and every probability SHALL be within 0.05

#### Scenario: A quantized decision graph loads and answers
- **WHEN** a model entry mounts an int8 decision export whose graph declares the five decision inputs
- **THEN** the model SHALL load and answer, and `EMB.INFO` SHALL report its quantization and on-disk size
