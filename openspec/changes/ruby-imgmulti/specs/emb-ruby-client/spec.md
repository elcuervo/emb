# Spec Delta

## ADDED Requirements

### Requirement: Ruby client accepts image calls in the multi block

The Ruby client SHALL accept `m[:model].image(bytes, *more, format:, outputs:)` inside `Emb.multi` and `Client#multi`, with the same arguments as `Proxy#image`. The block SHALL send the collected image pairs through `EMB.IMGMULTI`, and SHALL accept `format:` `:binary` (default) and `:values`. Results SHALL preserve submission order, one decoded Hash per image, and map a failed or truncated image to `nil`.

#### Scenario: Multi-model image results preserve order
- **WHEN** the block holds image calls for two models and both succeed
- **THEN** it returns one output-name-to-value map per image in submission order
- **AND** each map uses the reply decoder for the call's selected format

#### Scenario: A multi-image call yields one result per image
- **WHEN** `m[:model].image(a, b)` is called inside the block
- **THEN** both images are sent as ordered pairs and the block returns two Hashes, exactly as two separate calls would

#### Scenario: One failed image pair does not fail its siblings
- **WHEN** the server returns a null for one pair in an `EMB.IMGMULTI` reply
- **THEN** the corresponding result is `nil` and other results are decoded normally

#### Scenario: Text and image pairs interleave
- **WHEN** the block holds both text and image calls
- **THEN** text pairs are sent as `EMB.MULTI`, image pairs as `EMB.IMGMULTI`
- **AND** the returned results follow the block's submission order across both kinds

#### Scenario: Incompatible image calls are not merged
- **WHEN** image calls in one block request different formats or output projections
- **THEN** each compatible group is sent as its own `EMB.IMGMULTI` and each result uses its requested decoding

#### Scenario: Binary image bytes are preserved
- **WHEN** an image call receives bytes containing NUL or high-bit bytes
- **THEN** the client sends the exact bytes as binary-safe RESP bulk arguments without text transcoding or URL fetching

#### Scenario: VALUES format is decoded per image
- **WHEN** an image call inside the block uses `format: :values`
- **THEN** the command carries the leading `VALUES` keyword and each result carries its outputs' self-describing envelopes plus the pair's `model` tag

#### Scenario: Invalid format is rejected before sending
- **WHEN** an image call inside the block receives a format other than `:binary` or `:values`
- **THEN** it raises an argument error and sends no command

#### Scenario: The explicit block stays eager
- **WHEN** the client is configured with a lazy mode and image calls are collected in `Emb.multi`
- **THEN** the resulting `EMB.IMGMULTI` is sent during the block's `run`, not deferred
