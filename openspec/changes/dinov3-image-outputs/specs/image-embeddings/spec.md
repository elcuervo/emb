## MODIFIED Requirements

### Requirement: Per-model image input configuration

The server SHALL support an optional `image:` block on a model config declaring the image input tensor name, target size, crop mode, resample mode, rescale factor, per-channel mean, per-channel std, an optional `preset`, and an ordered `outputs` list. Each output SHALL declare a reply name and a graph tensor, plus an optional `select` (`none`, `cls`, `mean`, or `tokens`, defaulting to the model's `pooling`), an optional `start` (the first token row for `select: tokens`, default 0), and a a `grid` (the patch edge) that `select: tokens` requires, resolved from the output config, a preset, or the model's `config.json` metadata, so the emitted row count is always checkable. Output names SHALL be unique, and the list SHALL be the complete set of products the model serves; there is no default output. A model without an `image:` block SHALL NOT accept image commands and the server SHALL reply with an error naming the model. The server SHALL fail model loading, naming the offending output, when an output's `select` is unknown, when `select: tokens` is declared without `grid` or for a tensor whose rank is below 3, when `grid` is declared but `size` is not a multiple of it, or when a declared row count cannot be reconciled with the tensor and the configuration.

#### Scenario: CLIP-style preprocessing config accepted

- **WHEN** a model config declares `image: {input: pixel_values, size: 224, crop: center, mean: [0.48145466, 0.4578275, 0.40821073], std: [0.26862954, 0.26130258, 0.27577711], outputs: [{name: embedding, tensor: image_embeds}]}`
- **THEN** the server loads the model and image requests apply center-crop preprocessing with those constants, and a bare image request returns that output

#### Scenario: SigLIP-style preprocessing config accepted

- **WHEN** a model config declares `image: {input: pixel_values, size: 224, crop: none, mean: [0.5, 0.5, 0.5], std: [0.5, 0.5, 0.5], outputs: [{name: embedding, tensor: pooler_output}]}`
- **THEN** the server loads the model and image requests apply resize-without-crop preprocessing

#### Scenario: A token grid and a pooled vector from one model

- **WHEN** a model config declares `outputs: [{name: embedding, tensor: descriptor}, {name: patches, tensor: tokens, select: tokens, start: 5, grid: 21}]` at `size: 336`
- **THEN** the server loads the model with two outputs and validates that the token tensor can yield a 21x21 grid from row 5, and a bare request serves both

#### Scenario: A preset fills unnamed preprocessing fields

- **WHEN** a model config declares `image: {preset: dinov3, input: input, size: 336}` and the export ships no preprocessor config
- **THEN** mean, std, rescale, resample, crop, and token-layout defaults come from the named preset instead of the server's built-in fallback, and explicit keys still win over the preset

#### Scenario: Unusable output declaration is rejected

- **WHEN** an output declares an unknown `select`, `select: tokens` without `grid`, `select: tokens` on a rank-2 tensor, or a `grid` that does not divide `size`
- **THEN** model loading fails with an error naming the output and the conflicting values

#### Scenario: Layout constants come from the model's own metadata

- **WHEN** the model's `config.json` declares `patch_size: 16` and `num_register_tokens: 4` and the output config omits `start` and `grid`
- **THEN** the resolved token output starts at row 5 and expects a grid of `size/16` per side, and an explicit `start` or `grid` still wins

#### Scenario: Duplicate output names are rejected

- **WHEN** two outputs in one `image.outputs` list declare the same `name`
- **THEN** model loading fails with an error naming the duplicate

#### Scenario: Model without image config rejects EMB.IMG

- **WHEN** a client sends an image command for a model that has no `image:` block
- **THEN** the server replies with an error and performs no inference

### Requirement: Server-side image preprocessing

Given raw encoded image bytes and a model's image config, the server SHALL decode the image, resize and optionally crop to the configured size, rescale by the configured factor, normalize each channel with the configured mean and std, and produce a float32 tensor in `[1, 3, size, size]` RGB channel-first layout. Preprocessing SHALL be deterministic: identical bytes and config SHALL produce byte-identical tensors. Resolved preprocessing fields SHALL follow a fixed authority order: an explicit key in the model config, then `preprocessor_config.json` shipped beside the export, then the named `preset`, then the ONNX graph, then the built-in default. A size that none of those sources determines SHALL fail model loading with an error naming the model; every other field that falls through SHALL fall back to the built-in default, and the resolved plan SHALL be reported at load with the source of each field, so a built-in default is never silent.

#### Scenario: Output layout and dtype

- **WHEN** any decodable image is preprocessed for a model configured with `size: 224`
- **THEN** the produced tensor SHALL have shape `[1, 3, 224, 224]`, dtype float32, and RGB channel order

#### Scenario: Deterministic preprocessing

- **WHEN** the same image bytes are preprocessed twice with the same config
- **THEN** the two tensors SHALL be byte-identical

#### Scenario: Non-RGB input is converted

- **WHEN** a grayscale or RGBA image is preprocessed
- **THEN** the server SHALL convert it to RGB before normalizing

#### Scenario: Explicit keys override a preset

- **WHEN** a model config declares `preset: dinov3` together with `resample: bicubic`
- **THEN** the effective plan uses bicubic and the preset's other fields

#### Scenario: Resolved preprocessing is visible at load

- **WHEN** a model resolves a preprocessing plan
- **THEN** the server reports the resolved input, size, crop, resample, rescale, mean, and std, identifying the source family when a preset supplied them

### Requirement: Batched image inference

All images of a single image command (or `EMB.IMGMULTI` pair group) SHALL be preprocessed to the model's fixed tensor shape and inferred in batched session runs bounded by a per-chunk budget, so inference overhead does not grow linearly with the number of images while peak memory stays bounded. The chunk SHALL be sized so that neither the preprocessed input tensors nor the emitted outputs of a chunk exceed the budget, and it SHALL NOT exceed a batch dimension the graph declares statically. Preprocessing MAY be parallel; each chunk SHALL be a single batched call. Every requested output SHALL be read from that same call. An output whose batch dimension disagrees with the number of images it was computed from SHALL be a request error naming the mismatch: the server SHALL NOT pool, slice or read past an output tensor it did not get at the batch size it asked for. An output whose token rows disagree with the configured `start` and `grid` SHALL likewise be a request error naming both counts.

#### Scenario: N images, bounded batched inference

- **WHEN** a client sends N images for one model
- **THEN** the server SHALL execute session runs whose combined batch dimensions are N, using a single run when N fits the per-chunk budget and multiple bounded runs beyond it

#### Scenario: A static batch axis caps the chunk

- **WHEN** the model's graph declares a batch dimension of 1
- **THEN** every session run SHALL carry one image, and a multi-image request SHALL still serve every slot correctly

#### Scenario: Several outputs come from one run

- **WHEN** a request selects two outputs
- **THEN** the server SHALL execute one inference per chunk for both outputs, not one per output

#### Scenario: An output whose batch disagrees is refused

- **WHEN** a configured model's output tensor carries fewer (or more) rows than the images in the chunk that was run
- **THEN** the request SHALL fail with an error naming the expected and received batch dimensions
- **AND** the server SHALL NOT panic, and SHALL NOT read rows the output does not have

#### Scenario: A grid that does not match the tensor is refused

- **WHEN** a token output carries a row count that disagrees with the configured `start` and `grid`
- **THEN** the request SHALL fail with an error naming the configured and received row counts, and no partial grid is returned

### Requirement: Cross-modal dual-encoder pairing

When the same model serves both text and image embedding, `EMB <model> <text>` and every image output of that model SHALL carry vectors in the same dimensional space, using compatible pooling and normalization settings, so that text-to-image similarity is meaningful. Image outputs MAY read their own graph tensors (split `text_embeds`/`image_embeds` exports are supported), so output-tensor names need not match. The server SHALL fail model loading with a descriptive error when any image output's dimension disagrees with the text dimension.

#### Scenario: Shared space for text and image

- **WHEN** text and image are embedded with the same dual-encoder model
- **THEN** both embeddings SHALL have the model's configured dimension and identical normalization

#### Scenario: Dimension mismatch is rejected at load

- **WHEN** any image output's dimension differs from the model's text dimension
- **THEN** the server SHALL refuse to load the model with an error naming the output and both dimensions

### Requirement: Image embedding cache identity

Image vectors SHALL be cached under a content-addressed key derived from the model name, a hash of the source image bytes, and the output name (`img:<model>:<content-hash>:<output>`). Identical bytes and output SHALL hit regardless of how they were sent; different bytes, different outputs, and different models SHALL always miss. A request selecting several outputs SHALL consult and populate each output's entry independently, and one output's hit SHALL NOT suppress the inference its siblings need.

#### Scenario: Same bytes hit

- **WHEN** the same image bytes are requested twice for the same model and output
- **THEN** the second request SHALL be answered from the cache with no decode and no inference

#### Scenario: Changed bytes miss

- **WHEN** two different images are embedded for the same model
- **THEN** their cache keys differ and both are computed

#### Scenario: Different outputs do not share an entry

- **WHEN** the same image bytes are requested first for a pooled output and then for a token grid
- **THEN** the second request SHALL not be answered from the first entry, and both entries SHALL remain independently addressable

#### Scenario: Per-model accounting still works

- **WHEN** cache statistics are read after image requests
- **THEN** entries and bytes SHALL be attributed to the model name, and flushing one model SHALL remove that model's image entries for every output

### Requirement: EMB.IMG introspection

`EMB.HELP` SHALL document the image commands, the `OUTPUTS` keyword, and its position. `EMB.INFO <model>` SHALL report the model's configured image outputs with their name, graph tensor, selector, emitted row count or grid, and dimension. `emb.image.info()` SHALL report the same output list plus the resolved preprocessing plan. `EMB.STATS` SHALL count image requests and truncated images, and `MONITOR` SHALL record completed image requests with their item count.

#### Scenario: Outputs are discoverable before inference

- **WHEN** a client inspects a model that configures two image outputs
- **THEN** `EMB.INFO` SHALL list both outputs with their shapes, without running inference

#### Scenario: HELP lists the image commands

- **WHEN** a client sends `EMB.HELP`
- **THEN** the output SHALL include `EMB.IMG` and `EMB.IMGMULTI` with their syntax, including the `OUTPUTS` keyword and its fixed position

#### Scenario: Stats count image requests

- **WHEN** image requests have completed
- **THEN** `EMB.STATS` reports the image request and truncation counts

## REMOVED Requirements

### Requirement: EMB.IMG command

**Reason**: The image reply is now uniformly keyed by output name for both one and several images, replacing the single-bulk and positional-array shapes this requirement described. It is superseded rather than edited because its scenarios assert the old reply shape.

**Migration**: Clients read `reply["<output>"]` (a map under RESP3, a flat name/value array under RESP2) instead of a bare bulk or a positional array, and use `OUTPUTS <name>[,<name>...]` to narrow the projection. The replacement requirement is "EMB.IMG and keyed image replies".

## ADDED Requirements

### Requirement: EMB.IMG and keyed image replies

The server SHALL accept `EMB.IMG <model> [BLOB|VALUES] [OUTPUTS <name>[,<name>...]] <bytes>...` and reply, per image, a map from output name to value: raw little-endian float32 bytes under `BLOB`, a `dtype`/`shape`/`values` envelope under `VALUES`. One image SHALL reply one map and several SHALL reply an array of maps in request order. Omitting `OUTPUTS` SHALL serve every configured output, and both keywords SHALL be recognized only in their fixed positions and only when image arguments follow.

#### Scenario: Single binary image replies a keyed map

- **WHEN** a client sends `EMB.IMG siglip2 <cat.jpg bytes>` under the default BLOB format
- **THEN** the reply SHALL be one map whose single key is the model's configured output name and whose value is a bulk of the model-dimension float32 embedding

#### Scenario: Binary content is preserved

- **WHEN** an image whose bytes contain NUL, `0xFF`, or CR/LF sequences is sent as a bulk argument
- **THEN** the server receives the exact bytes and embeds them correctly

#### Scenario: Several images reply an array of keyed maps

- **WHEN** a client sends `EMB.IMG` with three image arguments
- **THEN** the reply SHALL be an array of three keyed maps in request order

#### Scenario: Several requested outputs reply a keyed map

- **WHEN** a client sends `EMB.IMG dinov3 OUTPUTS embedding,patches <bytes>` for a model configuring both outputs
- **THEN** the reply SHALL be one map keyed by `embedding` and `patches`, and the same request with two images SHALL reply an array of two such maps

#### Scenario: One requested output is still keyed

- **WHEN** a client sends `EMB.IMG dinov3 OUTPUTS patches <bytes>` for a model configuring several outputs
- **THEN** the reply SHALL be one map with the single key `patches`

#### Scenario: Format keyword position

- **WHEN** a client sends `EMB.IMG <model> VALUES <bytes>`
- **THEN** the keyword is interpreted as the reply format and the following argument is the first image

#### Scenario: Keyword positions are fixed

- **WHEN** a client sends `EMB.IMG <model> VALUES OUTPUTS embedding <bytes>`
- **THEN** `VALUES` is the reply format, `OUTPUTS` selects the projection, and the following argument is the first image

#### Scenario: Unknown output name is refused

- **WHEN** a client requests an output name the model does not configure
- **THEN** the command fails with an error naming the unknown name and the configured outputs, and no image is decoded

#### Scenario: Token grid envelopes carry their grid

- **WHEN** a token output is requested under `VALUES`
- **THEN** its envelope SHALL carry `shape` `[rows, dim]` and `grid` `[h, w]`

#### Scenario: Failed or truncated image replies null

- **WHEN** one image argument cannot be decoded, exceeds a limit, or falls past `max_images`
- **THEN** that position in the reply SHALL be null while the other images are served

#### Scenario: URL is rejected with guidance

- **WHEN** a client sends an `http(s)` or `data:` URL instead of image bytes
- **THEN** the server replies with an error stating that images must be fetched by the client and sent as bytes

### Requirement: EMB.IMGMULTI replies one keyed map per pair

The server SHALL accept `EMB.IMGMULTI [BLOB|VALUES] [OUTPUTS <name>[,<name>...]] <model> <bytes> [<model> <bytes>...]`, applying the same projection keyword at the same position, and SHALL reply one array with a keyed map per model/image pair in argument order. A pair that fails SHALL be null at its position, and under `VALUES` each pair's map SHALL carry the pair's `model` field beside its outputs.

#### Scenario: EMB.IMGMULTI replies keyed maps per pair

- **WHEN** a client sends `EMB.IMGMULTI OUTPUTS embedding,patches <model> <bytes> <model> <bytes>`
- **THEN** the reply SHALL be an array with one keyed map per pair in argument order, each carrying the same two keys, and `VALUES` SHALL add the pair's `model` field beside them

### Requirement: DINOv3-family export dialects load without re-export

The server SHALL serve both published DINOv3 ONNX dialects unmodified: exports with symbolic batch and spatial axes naming `last_hidden_state` and `pooler_output`, and exports pinned to one image naming `descriptor` and `tokens`. Both SHALL serve a pooled vector and a token grid from one inference per image. An external weights file SHALL be located from what the graph records, never from an assumed name. A pooled-only export SHALL serve without a token output.

#### Scenario: Dynamic-axis export loads and batches

- **WHEN** a model is configured against an export with symbolic axes, naming `pooler_output` and `last_hidden_state` at `size: 224`, and a request carries several images
- **THEN** every image SHALL be served from batched runs and the token output SHALL resolve to a 14x14 grid

#### Scenario: Static-batch export still serves several images

- **WHEN** a model is configured against an export whose input batch axis is fixed at 1
- **THEN** a request carrying several images SHALL still return one slot per image, executing one image per session run

#### Scenario: External weights are found beside the graph

- **WHEN** the graph records its weights in a sidecar file next to the model
- **THEN** the model SHALL load and run independently of the process working directory

#### Scenario: A pooled-only export needs no token output

- **WHEN** a model declares one rank-2 output
- **THEN** it SHALL serve that output, and requesting an unconfigured token output SHALL fail naming the configured outputs
