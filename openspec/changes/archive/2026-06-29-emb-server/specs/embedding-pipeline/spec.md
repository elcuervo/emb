## ADDED Requirements

### Requirement: Tokenization

The embedding pipeline SHALL convert input text to token IDs and attention masks using the model's tokenizer.

#### Scenario: Single text tokenization

- **WHEN** a single text string is processed
- **THEN** tokens are produced with `add_special_tokens=true`, truncated to `max_length` if longer, and padded to `max_length` if shorter

#### Scenario: Batch text tokenization

- **WHEN** multiple texts are processed in a single request
- **THEN** all texts are padded to `max(sequence_lengths)` within the batch, not to `max_length`. Attention masks distinguish real tokens from padding.

### Requirement: ONNX inference

The pipeline SHALL run the ONNX model with tokenized inputs and produce embedding vectors via the `DynamicAdvancedSession`.

#### Scenario: Single inference

- **WHEN** tokenized input tensors (input_ids, attention_mask) are fed to the session
- **THEN** the session executes `Run()` and produces a `last_hidden_state` tensor of shape `(batch, seq_len, hidden_dim)` with dtype float32

#### Scenario: Pre-allocated tensor reuse

- **WHEN** a worker processes consecutive requests
- **THEN** it SHALL reuse pre-allocated input and output tensors by overwriting data in place, avoiding allocation per request

### Requirement: Mean pooling

The pipeline SHALL compute the mean of token embeddings weighted by the attention mask, producing a single vector per text.

#### Scenario: Pooling with padding

- **WHEN** a batch contains texts of different lengths (padded)
- **THEN** the attention mask ensures padding tokens contribute zero to the mean. The sum is divided by the count of real tokens per text, not by `seq_len`.

### Requirement: L2 normalization

The pipeline SHALL L2-normalize pooled vectors so each has unit length.

#### Scenario: Normalized output

- **WHEN** a pooled embedding vector `v` is normalized
- **THEN** the output vector has `||v||₂ = 1` (within float32 precision)

### Requirement: Raw byte output

The pipeline SHALL return embeddings as little-endian float32 bytes, ready for RESP response writing.

#### Scenario: Output format

- **WHEN** a single embedding is produced
- **THEN** the output is `dim * 4` bytes, with each 4-byte group being a little-endian IEEE 754 float32

### Requirement: Error propagation

The pipeline SHALL propagate errors from tokenization, ONNX inference, or pooling to the caller.

#### Scenario: Tokenization failure

- **WHEN** tokenization fails (e.g., invalid input encoding)
- **THEN** the error is returned to the handler, which sends a RESP error to the client
