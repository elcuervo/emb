# tokenizer Specification

## Purpose
Specifies loading HuggingFace tokenizer.json files via the official tokenizers library.

## Requirements

### Requirement: HuggingFace tokenizer via official library

The server SHALL use the official `huggingface/tokenizers` Rust library (via `daulet/tokenizers` Go bindings) for all tokenization. The hand-rolled pure Go tokenizer is removed.

#### Scenario: Loads any HuggingFace tokenizer.json

- **WHEN** a tokenizer JSON file is loaded
- **THEN** the server SHALL support WordPiece, BPE, and Unigram model types
- **THEN** the server SHALL match the output of the `huggingface/tokenizers` Python library for identical inputs

#### Scenario: Tokenizer interface unchanged

- **WHEN** the pipeline calls `Encode(text, maxLength)`
- **THEN** it SHALL return `[]int64` input IDs, `[]int64` attention mask, and error
- **THEN** input IDs SHALL include special tokens (CLS/SEP for WordPiece) when the tokenizer adds them
- **THEN** sequences SHALL be truncated to `maxLength`

#### Scenario: Embeddings match upstream reference

- **WHEN** the server encodes text through the new tokenizer and runs inference
- **THEN** the output embeddings SHALL have cosine similarity > 0.9999 compared to a Python reference using the same model and `sentence-transformers` library

### Requirement: Special-token IDs and plain encoding for sequence builders

The tokenizer SHALL expose the model's `[MASK]`, `[CLS]`, `[SEP]`, and `[PAD]` token IDs, resolved from the tokenizer's own definitions (in order of preference: the tokenizer configuration's special-token names, then the `added_tokens` entries in `tokenizer.json`, then single-token encoding probes), matching the Ruby gem's special-token discovery semantics. It SHALL also expose an encode variant that returns token IDs without special tokens — `encode_ids(text)` — with the same output the Hugging Face tokenizers library produces for `encode(text, add_special_tokens=False)`. The existing `Encode(text, maxLength)` behavior (special tokens included, truncated) SHALL be unchanged.

#### Scenario: Special-token IDs resolve from the tokenizer file

- **WHEN** a checkpoint's `tokenizer.json` declares `[MASK]`, `[CLS]`, `[SEP]`, `[PAD]` as added tokens
- **THEN** the server exposes their token IDs without running a probe encode, and the IDs match the entries' ids

#### Scenario: Plain encode matches the Python reference

- **WHEN** a text is encoded with `encode_ids`
- **THEN** the IDs equal `encode(text, add_special_tokens=False)` from the official tokenizers library for the same file, and `Encode` still includes special tokens
