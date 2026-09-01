# inference-performance Specification

## Purpose
Specifies that `emb` MAY execute embedding post-processing (L2 normalization, mean/CLS pooling, float32 little-endian marshalling) outside the scalar Go loop — for both 2D pre-pooled models and 3D-output models — and MAY tune ONNX inference sessions and the batching window for concurrent multi-model serving, while preserving retrieval-correct embeddings (cosine ≥ 0.99 of the fp32 baseline) and the fp32 wire format.

## Requirements
### Requirement: Fast pre-pooled output path

For a pre-pooled (2D-output, `pooling: none`) model, the server SHALL return the model's pooled embedding as little-endian float32 bytes of length `dim * 4` per text, and MAY do so with minimal work (reused buffers and zero-copy marshalling). When normalization is disabled and pooling is baked into the graph, the path SHALL perform no normalization and no pooling arithmetic.

#### Scenario: Pre-pooled model returns correct bytes

- **WHEN** a model with a pre-pooled 2D output (e.g. `siglip2`, `text_embeds` at `dim` 768) is served with `pooling: none`
- **THEN** each text returns a little-endian float32 vector of length `dim * 4`

#### Scenario: Pooled-in-graph model performs no math

- **WHEN** a model with pooling and output layers baked into the graph and `normalize: false` (e.g. the custom `e5`) is served
- **THEN** the returned bytes SHALL be the model's pooled output marshalled with no additional normalization or pooling arithmetic in the server

#### Scenario: Fast path within tolerance of reference

- **WHEN** the same text is embedded via the fast pre-pooled path and via the reference scalar path
- **THEN** mean cosine between the two SHALL be ≥ 0.99, and the byte length SHALL be `dim*4`

### Requirement: Accelerated numeric normalization

The server SHALL provide an optimized (row-parallel and/or SIMD) path for L2-normalization of pre-pooled embeddings that returns the same `dim * 4` little-endian float32 format and stays within cosine ≥ 0.99 of the scalar fp32 result.

#### Scenario: Optimized L2 within tolerance

- **WHEN** a normalized pre-pooled model (e.g. `siglip2`, `normalize: true`) is served
- **THEN** each returned vector SHALL be length `dim * 4` little-endian float32
- **AND** mean cosine vs the scalar fp32 result SHALL be ≥ 0.99

### Requirement: Fast 3D pooling path

For a 3D-output model (e.g. `minilm`, `bge`) the server SHALL provide a fast host-side pooling path (mean or CLS over the attention mask plus optional L2 normalization) that returns `dim * 4` little-endian float32 bytes and stays within cosine ≥ 0.99 of the scalar fp32 result. The server MAY alternatively compute pooling inside the ONNX graph, with the same output contract.

#### Scenario: 3D model pooled fast within tolerance

- **WHEN** a model with 3D `last_hidden_state` output (e.g. `minilm`) is served
- **THEN** each text returns a length-`dim*4` little-endian float32 vector
- **AND** mean cosine vs the scalar fp32 result SHALL be ≥ 0.99

#### Scenario: Pooling-in-graph returns final vector

- **WHEN** a 3D model is served with graph-side pooling (mean-pool + L2 in the ONNX graph)
- **THEN** one inference run SHALL return the final length-`dim*4` little-endian float32 vector per text

### Requirement: Concurrent multi-model session tuning

The server SHALL allow tuning ONNX session execution (execution mode and intra-op thread count) per model so two models (e.g. `siglip2` and `e5`) serving concurrently do not unnecessarily contend for CPU, without changing the returned embeddings.

#### Scenario: Two models served concurrently

- **WHEN** an `EMB.MULTI` embeds the same text through two models
- **THEN** both models SHALL return correct, length-`dim*4` embeddings
- **AND** session options SHALL be configurable per model (execution mode and `intra_op_threads`)
