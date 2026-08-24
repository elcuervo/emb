# pad-output Specification

## Purpose
Specifies the `pad_output` config option for legacy-model compatibility (pad sequences to max_length with trailing zeros) and tokenizer configuration.

## Requirements
### Requirement: pad_output config option

The server SHALL support a per-model `pad_output` configuration option to control whether tokenized sequences are padded to `max_length`.

#### Scenario: Default behavior (pad_output: false)

- **WHEN** a model config does not specify `pad_output` or sets it to `false`
- **THEN** the tokenizer SHALL strip trailing [PAD] tokens from encoded sequences
- **THEN** the attention mask SHALL have 1s only for real token positions
- **THEN** the pipeline SHALL pad sequences to the batch max length and set the attention mask correctly for ONNX inference

#### Scenario: Legacy compatibility (pad_output: true)

- **WHEN** a model config sets `pad_output: true`
- **THEN** the tokenizer SHALL NOT strip trailing [PAD] tokens
- **THEN** the tokenizer SHALL pad sequences to `max_length` with zeros
- **THEN** the attention mask SHALL have 1s for all positions (including padding)
- **THEN** the ONNX inference SHALL produce embeddings identical to Ruby `Siglip2Text` output

### Requirement: Tokenizer configuration

The tokenizer SHALL receive the model's `pad_output` flag so padding behavior is applied consistently at tokenization.

#### Scenario: Model with tokenizer path

- **WHEN** a model loads a tokenizer via `tokenizer.NewTokenizer`
- **THEN** the `padOutput` parameter SHALL control whether padding is stripped
