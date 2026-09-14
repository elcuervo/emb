# script-embed Specification

## Purpose
Gives sandboxed scripts first-class access to embeddings and vector scoring: pooled normalized vectors through the model's own embedding path, symmetric image embeddings for cross-modal models, and similarity/distance methods over vectors or packed bytes.

## Requirements

### Requirement: Pooled embedding primitive

The server SHALL provide `emb.embed` to scripted evaluations, returning the model's **pooled, normalized** embedding — the same vectors the `EMB` command returns, produced by the same embedding pipeline (tokenization, pooling, normalization), never by script-side tensor math.

`emb.embed(text)` SHALL return one vector; `emb.embed({text1, text2, ...})` SHALL return an array of vectors in input order. A vector SHALL be a Lua array of `dim` numbers in the model's declared dimension.

`emb.embed` SHALL be available only for models the server can embed (a declared dimension and a pooling mode other than `none`); for models without an embedding configuration the function SHALL be absent, so scripts written for non-embedding graphs are unaffected. Text-count and tensor-element budgets SHALL apply.

#### Scenario: Single text returns one vector

- **WHEN** a script calls `emb.embed("hello")` on a model with dimension 384
- **THEN** the result is an array of 384 numbers equal to the vector `EMB <model> hello` returns for the same text

#### Scenario: Batch form returns vectors in order

- **WHEN** a script calls `emb.embed({"alpha", "beta"})`
- **THEN** the result is a 2-element array whose first and second vectors equal the embeddings of `alpha` and `beta` respectively

#### Scenario: Absent for non-embeddable models

- **WHEN** a script on a model with no embedding configuration calls `emb.embed`
- **THEN** the evaluation fails with an error stating the function is unavailable for that model, and no model state is changed

#### Scenario: Determinism and cacheability

- **WHEN** the same script, args, and text are evaluated twice
- **THEN** `emb.embed` returns byte-identical vectors and the reply is served from the content-addressed script cache on the second evaluation

### Requirement: Embedding work is shared with the native path

`emb.embed` SHALL execute through the model's embedding pool so that its inference is subject to the same batching and caching as `EMB`:

- Texts SHALL be submitted to the same batcher as `EMB` requests, allowing concurrent scripted and native requests to coalesce into a single inference run.
- Results SHALL be read from and written to the same content-addressed embedding cache the `EMB` command uses, so a text embedded by either path is a cache hit for the other.
- The scripted path SHALL NOT open a separate inference session pool to satisfy `emb.embed`.

#### Scenario: Native and scripted embeddings share a cache entry

- **WHEN** a server with caching enabled serves `EMB <model> hello` and a script then evaluates `emb.embed("hello")`
- **THEN** the scripted call is served from the existing cache entry and the script cache-hit path records no new inference

#### Scenario: No extra sessions for embed-only scripts

- **WHEN** a model has never executed `emb.run` and a script using only `emb.embed` is evaluated
- **THEN** the model's named-tensor script sessions remain unopened

#### Scenario: Concurrent coalescing

- **WHEN** a scripted `emb.embed` and an `EMB` request for distinct texts arrive concurrently
- **THEN** both are admitted to the same batcher and may be satisfied by one inference run

### Requirement: Packed vector form

`emb.embed` SHALL accept a flag requesting the packed form: `emb.embed(texts, {bytes = true})` SHALL return each vector as a Lua string of `dim` little-endian float32 values (4 bytes per element), and `emb.embed(text, {bytes = true})` SHALL return one such string. The packed form SHALL be byte-compatible with `emb.math.float32_bytes` output and with the `bytes` input field of `emb.run`.

#### Scenario: Packed vector round-trips

- **WHEN** a script packs `emb.embed("hello")` with `emb.math.float32_bytes` and embeds `"hello"` with `{bytes = true}`
- **THEN** the two Lua strings are byte-identical

#### Scenario: Packed vectors feed similarity

- **WHEN** a script passes two `{bytes = true}` results to `emb.similarity`
- **THEN** the returned score equals the score computed from the same vectors in array form

#### Scenario: Packed length is validated

- **WHEN** a packed value whose length is not a multiple of 4, or not equal to `dim × 4` bytes, is passed to a vector operation
- **THEN** the evaluation fails with a length error naming the expected size

### Requirement: Image embedding primitive

For models with an image configuration, the server SHALL provide `emb.image.embed(bytes)` and `emb.image.embed({bytes1, ...})`, returning pooled, normalized image embeddings from the model's image branch in the same embedding space as `emb.embed`. Each argument SHALL be the raw encoded bytes of a JPEG, PNG, GIF, or WebP image; URLs SHALL be rejected, mirroring `EMB.IMG`. The packed form SHALL be available as `{bytes = true}`.

#### Scenario: Image vector matches EMB.IMG

- **WHEN** a script calls `emb.image.embed(imageBytes)` and the same bytes are sent to `EMB.IMG <model>`
- **THEN** the returned vector equals the `EMB.IMG` reply for the same bytes

#### Scenario: Cross-modal scoring

- **WHEN** a dual-encoder model serves a script that embeds a text with `emb.embed` and an image with `emb.image.embed` and scores them with `emb.similarity`
- **THEN** the returned value is the cosine similarity of the two embeddings in the shared space

#### Scenario: URLs are rejected

- **WHEN** a script passes an `https://` string to `emb.image.embed`
- **THEN** the evaluation fails with an error stating images must be supplied as bytes, without fetching anything

### Requirement: Similarity method

The server SHALL provide `emb.similarity(a, b [, metric])`. The result SHALL be a number where a **larger value means more similar**, for every supported metric:

- `cosine` (default): cosine similarity in `[-1, 1]`
- `dot`: inner product

Both operands SHALL each be either a Lua array of numbers or a packed float32 byte string. Operands of different lengths SHALL be an error. Empty operands SHALL be an error. An unknown metric name SHALL be an error naming the accepted metrics. A zero-magnitude operand SHALL yield `0` for `cosine` rather than an error.

#### Scenario: Cosine is the default

- **WHEN** a script calls `emb.similarity(a, b)` with two vectors
- **THEN** the result equals `emb.similarity(a, b, "cosine")`

#### Scenario: Identical vectors score one

- **WHEN** identical non-zero vectors are passed to `emb.similarity`
- **THEN** the result is `1` (within float tolerance)

#### Scenario: Dot is the inner product

- **WHEN** `emb.similarity(a, b, "dot")` is called
- **THEN** the result equals the sum of element-wise products

#### Scenario: Mixed operand forms are accepted

- **WHEN** one operand is an array of numbers and the other is a packed float32 string of the same dimension
- **THEN** the call succeeds and returns the same value as with two arrays

#### Scenario: Length mismatch errors

- **WHEN** operands have different element counts
- **THEN** the evaluation fails with an error naming both lengths

#### Scenario: Zero vector scores zero

- **WHEN** either operand is all zeros and the metric is `cosine`
- **THEN** the result is `0`, not an error

### Requirement: Distance method

The server SHALL provide `emb.distance(a, b [, metric])`. The result SHALL be a number where a **smaller value means closer**, for every supported metric:

- `l2` (default): Euclidean distance
- `l2sq`: squared Euclidean distance
- `cosine`: cosine distance, defined as `1 − cosine similarity`

Operand forms, length validation, empty-operand handling, and unknown-metric errors SHALL match `emb.similarity`.

#### Scenario: L2 is the default

- **WHEN** a script calls `emb.distance(a, b)` with two vectors
- **THEN** the result equals `emb.distance(a, b, "l2")`

#### Scenario: Cosine distance complements similarity

- **WHEN** `emb.distance(a, b, "cosine")` and `emb.similarity(a, b, "cosine")` are called on the same vectors
- **THEN** the two results sum to `1` within float tolerance

#### Scenario: Squared L2 is the square of L2

- **WHEN** `emb.distance(a, b, "l2sq")` is called
- **THEN** the result equals the square of `emb.distance(a, b, "l2")` within float tolerance

#### Scenario: Identical vectors are zero distance

- **WHEN** identical vectors are passed to `emb.distance`
- **THEN** the result is `0`
