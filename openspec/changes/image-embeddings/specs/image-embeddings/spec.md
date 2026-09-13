## Purpose

Serves embedding vectors for images without the client decoding, resizing, normalizing, or serializing pixel data, by resolving raw image bytes to the model's expected `pixel_values` tensor server-side.

## ADDED Requirements

### Requirement: Per-model image input configuration

The server SHALL support an optional `image:` block on a model config declaring the image input tensor name, target size, crop mode, resample mode, rescale factor, per-channel mean, and per-channel std, so both CLIP-family (center-crop, OpenAI mean/std) and SigLIP-family (no crop, mean/std 0.5) preprocessing are expressible without code changes. A model without an `image:` block SHALL NOT accept `EMB.IMG` and the server SHALL reply with an error naming the model.

#### Scenario: CLIP-style preprocessing config accepted

- **WHEN** a model config declares `image: {input: pixel_values, size: 224, crop: center, mean: [0.48145466, 0.4578275, 0.40821073], std: [0.26862954, 0.26130258, 0.27577711]}`
- **THEN** the server loads the model and `EMB.IMG` applies center-crop preprocessing with those constants

#### Scenario: SigLIP-style preprocessing config accepted

- **WHEN** a model config declares `image: {input: pixel_values, size: 224, crop: none, mean: [0.5, 0.5, 0.5], std: [0.5, 0.5, 0.5]}`
- **THEN** the server loads the model and `EMB.IMG` applies resize-without-crop preprocessing

#### Scenario: Model without image config rejects EMB.IMG

- **WHEN** a client sends `EMB.IMG` for a model that has no `image:` block
- **THEN** the server replies with an error and performs no inference

### Requirement: Server-side image preprocessing

Given raw encoded image bytes and a model's image config, the server SHALL decode the image, resize and optionally crop to the configured size, rescale by the configured factor, normalize each channel with the configured mean and std, and produce a float32 tensor in `[1, 3, size, size]` RGB channel-first layout. Preprocessing SHALL be deterministic: identical bytes and config SHALL produce byte-identical tensors.

#### Scenario: Output layout and dtype

- **WHEN** any decodable image is preprocessed for a model configured with `size: 224`
- **THEN** the produced tensor SHALL have shape `[1, 3, 224, 224]`, dtype float32, and RGB channel order

#### Scenario: Deterministic preprocessing

- **WHEN** the same image bytes are preprocessed twice with the same config
- **THEN** the two tensors SHALL be byte-identical

#### Scenario: Non-RGB input is converted

- **WHEN** a grayscale or RGBA image is preprocessed
- **THEN** the server SHALL convert it to RGB before normalizing

### Requirement: EMB.IMG command

The server SHALL accept `EMB.IMG <model> [BLOB|VALUES] <bytes> [<bytes>...]`, where each `<bytes>` is the raw encoded content of an image (for example the bytes of a JPEG, PNG, GIF, or WebP file) sent as a binary-safe RESP bulk. The server SHALL decode and preprocess every argument, run one batched inference, and reply with one slot per argument. The optional format keyword SHALL be recognized only at position 2 (immediately after the model) and only when at least one image argument follows. The server SHALL NOT fetch remote URLs, and an argument that is a URL rather than image bytes SHALL fail with an error directing the client to fetch the image itself.

#### Scenario: Single binary image replies one bulk

- **WHEN** a client sends `EMB.IMG siglip2 <cat.jpg bytes>` under the default BLOB format
- **THEN** the reply SHALL be a single bulk containing the model-dimension float32 embedding

#### Scenario: Binary content is preserved

- **WHEN** an image whose bytes contain NUL, `0xFF`, or CR/LF sequences is sent as a bulk argument
- **THEN** the server receives the exact bytes and embeds them correctly

#### Scenario: Multiple images reply one slot per image

- **WHEN** a client sends `EMB.IMG` with three image arguments
- **THEN** the reply SHALL be an array of three embeddings in request order

#### Scenario: Format keyword position

- **WHEN** a client sends `EMB.IMG <model> VALUES <bytes>`
- **THEN** the keyword is interpreted as the reply format and the following argument is the first image

#### Scenario: URL is rejected with guidance

- **WHEN** a client sends an `http(s)` or `data:` URL instead of image bytes
- **THEN** the server replies with an error stating that images must be fetched by the client and sent as bytes

### Requirement: Batched image inference

All images of a single `EMB.IMG` (or `EMB.IMGMULTI` pair group) SHALL be preprocessed to the model's fixed tensor shape and inferred in batched session runs bounded by a fixed per-chunk tensor budget, so inference overhead does not grow linearly with the number of images while peak memory stays bounded. Preprocessing MAY be parallel; each chunk SHALL be a single batched call.

#### Scenario: N images, bounded batched inference

- **WHEN** a client sends `EMB.IMG` with N images for one model
- **THEN** the server SHALL execute session runs whose combined batch dimensions are N, using a single run when N fits the per-chunk budget and multiple bounded runs beyond it

### Requirement: Cross-modal dual-encoder pairing

When the same model serves both text and image embedding, `EMB <model> <text>` and `EMB.IMG <model> <image>` SHALL emit vectors in the same dimensional space, using compatible dimensions plus the model's pooling and normalization settings, so that text-to-image similarity is meaningful. The image branch MAY read its own output tensor via `image.output` (split `text_embeds`/`image_embeds` exports are supported), so output-tensor names need not match. The server SHALL fail model loading with a descriptive error when the text and image output dimensions disagree.

#### Scenario: Shared space for text and image

- **WHEN** text and image are embedded with the same dual-encoder model
- **THEN** both embeddings SHALL have the model's configured dimension and identical normalization

#### Scenario: Dimension mismatch is rejected at load

- **WHEN** a model's text output and image output have different dimensions
- **THEN** the server SHALL refuse to load the model with an error naming both dimensions

### Requirement: Image request limits and truncation

The server SHALL bound the number of images processed per command (`max_images`, default 4096 when unset, `0` = unlimited), the byte size of each image argument, and the decoded pixel count of each image. Images beyond `max_images` SHALL be truncated: the reply SHALL have one slot per requested image with the processed prefix populated and the remainder `null`, and overflow images SHALL NOT be decoded, inferred, or cached. A single undecodable or over-limit image SHALL fail only its own slot.

#### Scenario: Oversized command is truncated

- **WHEN** a client sends more images than `max_images`
- **THEN** only the first `max_images` are processed and the remaining reply slots are null

#### Scenario: One bad image does not fail the command

- **WHEN** one argument in a multi-image request fails to decode or exceeds the byte/pixel cap
- **THEN** that slot is null/error and the remaining images still return embeddings

### Requirement: Image embedding cache identity

Image embeddings SHALL be cached under a content-addressed key derived from the model name and a hash of the source image bytes (`img:<model>:<content-hash>`). Identical bytes SHALL hit regardless of how they were sent; different bytes SHALL always miss.

#### Scenario: Same bytes hit

- **WHEN** the same image bytes are embedded twice for the same model
- **THEN** the second request SHALL hit the cache under the same content-hash key

#### Scenario: Changed bytes miss

- **WHEN** two different images are embedded for the same model
- **THEN** their cache keys differ and both are computed

### Requirement: EMB.IMG introspection

`EMB.HELP` SHALL document `EMB.IMG` and `EMB.IMGMULTI`. `EMB.STATS` SHALL count image requests and truncated images, and `MONITOR` SHALL record completed image requests with their item count.

#### Scenario: HELP lists the image commands

- **WHEN** a client sends `EMB.HELP`
- **THEN** the output SHALL include `EMB.IMG` and `EMB.IMGMULTI` with their syntax

#### Scenario: Stats count image requests

- **WHEN** image requests have completed
- **THEN** `EMB.STATS` reports the image request and truncation counts
