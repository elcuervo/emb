## ADDED Requirements

### Requirement: Special-token IDs and plain encoding for sequence builders

The tokenizer SHALL expose the model's `[MASK]`, `[CLS]`, `[SEP]`, and `[PAD]` token IDs, resolved from the tokenizer's own definitions (in order of preference: the tokenizer configuration's special-token names, then the `added_tokens` entries in `tokenizer.json`, then single-token encoding probes), matching the Ruby gem's special-token discovery semantics. It SHALL also expose an encode variant that returns token IDs without special tokens — `encode_ids(text)` — with the same output the Hugging Face tokenizers library produces for `encode(text, add_special_tokens=False)`. The existing `Encode(text, maxLength)` behavior (special tokens included, truncated) SHALL be unchanged.

#### Scenario: Special-token IDs resolve from the tokenizer file

- **WHEN** a checkpoint's `tokenizer.json` declares `[MASK]`, `[CLS]`, `[SEP]`, `[PAD]` as added tokens
- **THEN** the server exposes their token IDs without running a probe encode, and the IDs match the entries' ids

#### Scenario: Plain encode matches the Python reference

- **WHEN** a text is encoded with `encode_ids`
- **THEN** the IDs equal `encode(text, add_special_tokens=False)` from the official tokenizers library for the same file, and `Encode` still includes special tokens