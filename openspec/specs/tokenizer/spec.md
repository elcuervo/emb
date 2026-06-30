## ADDED Requirements

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
