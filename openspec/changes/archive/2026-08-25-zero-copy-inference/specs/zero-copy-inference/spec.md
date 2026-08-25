# zero-copy-inference Specification

## Purpose
Specifies pooled tensor buffers and borrowed-output semantics so inference runs avoid per-request allocation and a full output copy on ARM64/Graviton CPU serving.

## ADDED Requirements

### Requirement: Buffers are pooled and reused
The inference path SHALL reuse input-tensor backing buffers across runs instead of allocating fresh arrays per request.

#### Scenario: Input buffer reuse
- **WHEN** two inference runs execute for the same model
- **THEN** input_ids and attention_mask backing arrays SHALL be drawn from and returned to a pool rather than freshly allocated each run
- **THEN** token contents SHALL be copied into the pooled buffers so no stale values leak

#### Scenario: Output buffer handed off, not copied
- **WHEN** an ONNX run completes
- **THEN** the output tensor's backing slice SHALL be the only copy of the float32 data, returned to the caller with ownership

### Requirement: Buffer lifetime and release
Response buffers SHALL remain valid until the RESP reply is written and SHALL then be returned to the pool.

#### Scenario: Reply-then-release
- **WHEN** a response is written to the connection
- **THEN** its embeddings SHALL be released back to the pool by the server after the write returns, never while the write is in progress

### Requirement: Identical output bytes
Pooling and buffer reuse SHALL NOT alter embedding values.

#### Scenario: Byte-identical results
- **WHEN** the same texts are embedded before and after pooling is enabled
- **THEN** the returned float32 bytes SHALL be identical