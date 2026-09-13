# Image Embeddings

## Why

`emb` serves text embeddings over RESP, but multimodal retrieval (product search,
visual dedup, image-to-image similarity, RAG over PDFs/screenshots) needs vectors
for **images**, and today the only way to get one is to have the *client* fetch,
decode, resize, normalize, and ship a `pixel_values` tensor through RESP — i.e.
"convert the image to text" by hand. Every client re-implements CLIP/SigLIP
preprocessing, and every one of them can drift from the reference pipeline.

The scripting layer already proves the serving machinery is there
(`onnx.NamedRuntimeSession`, `emb.run`, and `examples/scripts/siglip2.lua`, which
already builds a `pixel_values` tensor for a fused CLIP export — with zeros,
because no image decoder exists). This change adds the missing primitive —
**server-side image preprocessing** — and a first-class `EMB.IMG` / `EMB.IMGMULTI`
surface that accepts raw image bytes directly over RESP.

RESP bulk strings are binary-safe, so an image needs no base64, no URL, and no
text encoding: the client sends the file's bytes and the server does the rest.

## What Changes

- **New `EMB.IMG <model> [BLOB|VALUES] <bytes> [<bytes>...]`**: embed one or more
  images per model, where each `<bytes>` is the raw encoded content of an image
  (JPEG/PNG/GIF/WebP) sent as a binary-safe RESP bulk. Decode → resize → rescale →
  normalize → CHW `pixel_values` → one batched inference → the exact existing
  `BLOB`/`VALUES` reply grammar. Works directly with `redis-cli -x EMB.IMG <model>`.
- **New `EMB.IMGMULTI [BLOB|VALUES] <model> <bytes> [<model> <bytes>...]`**:
  cross-model image embedding with `EMB.MULTI`'s MGET-style per-pair null semantics.
- **Per-model `image:` config block** (input tensor, `size`, `crop`, `resample`,
  `rescale`, `mean`, `std`, output tensor, pooling, normalize) so CLIP and SigLIP
  preprocessing variants are expressible without code changes, auto-detected from
  HF `preprocessor_config.json` and the ONNX `pixel_values` shape where possible.
- **Content-addressed image caching**: `img:<model>:sha256(bytes)` keys, so a
  changed image is a different key — never stale — and a hit costs no decode and no
  network.
- **Bounded command size**: a configurable `max_command_bytes` (with a per-image
  byte cap and decoded-pixel cap) so large binary payloads cannot make the server
  allocate unbounded memory for a single command.
- **Dual-encoder pairing enforcement**: a model serving both `EMB` and `EMB.IMG`
  must use compatible dimensions, pooling, and normalization (separate per-branch
  output tensors are supported), so text-to-image retrieval is meaningful.
- **Scripting works with images**: the Lua sandbox gains `emb.image.preprocess(bytes)`
  (and `emb.image.info()`) so scripts can decode/preprocess image bytes and feed the
  resulting tensor to `emb.run`, enabling zero-shot classification and cross-modal
  ranking. This requires a packed `bytes` tensor input form (so a 150k-element tensor
  never round-trips through a Lua table) and a bounded script reply-cache key (so an
  image-sized KEYS element does not inline megabytes into the key).
- **Documentation updates**: README command and script-surface tables, `EMB.HELP`,
  the config reference, and a runnable image example script — all updated as part of
  this change, including the preprocessing-parity caveat.

**Explicitly out of scope (deliberately):**

- **Remote URL fetching.** The server SHALL NOT fetch images from URLs. Fetching is
  a client concern (credentials, retries, allowlists, rate limits, SSRF),
  and the URL was never required for the goal — raw bytes over RESP *is* "no image
  to text". This keeps the request path network-free and removes an entire egress
  and SSRF surface.
- **Script HTTP / remote fetch in Lua.** Scripts receive image *bytes* (via KEYS)
  and preprocess them host-side; the sandbox stays network-free, so script replies
  remain cacheable and deterministic. If a general remote-fetch capability is ever
  needed, it belongs in its own change with its own security review, not here.
- Interleaved text+image inputs, video, OCR, reranking, indexing, and vector storage.
- Tensor input (a client-preprocessed `pixel_values` buffer). Raw encoded bytes are
  the supported input; clients that genuinely need tensor input already have
  `EMB.EVAL` + `emb.run`.

**BREAKING:** none. All new config keys are optional; existing text models and
commands are unchanged.

## Capabilities

### New Capabilities

- `image-embeddings`: per-model image input configuration, server-side decode /
  resize / rescale / normalize preprocessing, and the `EMB.IMG` command including
  batching, reply formats, limits, and content-addressed caching.

### Modified Capabilities

- `emb-multi`: add `EMB.IMGMULTI` cross-model image embedding with `EMB.MULTI`'s
  per-pair null/MGET semantics, truncation, and stats accounting.
- `embedding-reply-format`: extend the `BLOB|VALUES` keyword grammar to `EMB.IMG`
  and `EMB.IMGMULTI` (keyword position, `VALUES` shape semantics for image results).
- `lru-cache`: define image entry keys (`img:<model>:<content-hash>`), their
  participation in byte budgets/eviction, and model-scoped invalidation.
- `model-autoconfig`: auto-detect the image input tensor and preprocessing
  parameters (size, mean/std, rescale, crop, resample) from the ONNX graph and
  `preprocessor_config.json`, and require a text/image dual-encoder pairing for
  cross-modal use.
- `request-size-guardrails`: add a `max_command_bytes` bound (plus a per-image byte
  cap) so binary image payloads cannot exhaust server memory.
- `script-eval`: expose `emb.image.preprocess`/`emb.image.info` in the sandbox, make
  binary KEYS explicit, and bound reply-cache key size for large payloads.
- `script-tensor-utils`: add a packed `bytes` tensor input form so host-produced
  tensors (image pixels) reach `emb.run` without a per-element Lua table.

## Impact

- **New package:** `internal/imageproc` (decode + resize + normalize, stdlib
  `image/png|jpeg|gif` plus `golang.org/x/image/*`). No `net/http` in the request path.
- **Config:** `ModelConfig` gains a nested `image:` block; top-level config gains
  `max_command_bytes` and a per-image byte cap. No existing key changes.
- **Server:** new `handleIMG` / `handleIMGMULTI` and dispatch entries; `EMB.HELP`,
  `EMB.STATS`, and `MONITOR` gain image counters; `CONFIG GET/SET` gains the byte
  caps. The connection read path needs a command-size limit (either a small
  redcon-fork addition or a bounded connection reader).
- **Registry:** `ModelEntry` gains an image resource pool (named sessions) and a
  preprocessing plan; the model fingerprint gains the image config so snapshots
  invalidate correctly.
- **Scripting:** `internal/script` gains the `emb.image` host block (backed by
  `internal/imageproc`), the packed-`bytes` tensor input form, and a
  digest-based `script.CacheKey` for large payloads. `Hosts` gains an image-
  preprocessing binding.
- **Dependencies:** `golang.org/x/image` (currently not in `go.mod`). No CGo or
  ONNX Runtime changes — image tensors reuse the existing named-tensor path.
- **Docs:** README command and script-surface tables, `EMB.HELP`, config reference,
  and a runnable image example script (`examples/scripts/`).
- **Ruby client:** `gems/emb` gains an image embedding method that passes bytes
  as an `ASCII-8BIT` string unchanged; the existing float32 reply decode is reused.
- **Tests/docs:** golden preprocessing parity fixture vs. a Python/PIL reference,
  binary-safety and byte-cap tests, config/autoconfig tests, `EMB.IMG` end-to-end
  against a real SigLIP2 vision export, and README/`EMB.HELP` documentation.
