# Design: Image Embeddings

## Context

See `proposal.md` — Why for motivation. This section records the current state that
shapes the approach.

- **Serving machinery already exists.** `internal/onnx/named.go` (`NamedSession` /
  `RunNamed`) accepts arbitrary named float32/int64 tensors, and the scripted-model
  path (`internal/registry/registry.go` `ScriptResources`) already pools named
  sessions. `examples/scripts/siglip2.lua` already constructs a `[1,3,224,224]`
  `pixel_values` tensor and reads `text_embeds` from a fused CLIP export — it fills
  the image branch with **zeros** because nothing decodes images.
- **What is missing is exactly one thing:** image decode/preprocess.
- **RESP is binary-safe.** `redcon`'s parser reads `$<len>\r\n` and slices exactly
  `len` bytes (`redcon.go` `readCommands`), so `cmd.Args [][]byte` carries arbitrary
  bytes. `redis-cli -x` reads the last argument from stdin, so
  `cat cat.jpg | redis-cli -x EMB.IMG <model>` works with no encoding step.
- **The Lua sandbox is deliberately network-free.** `internal/script/engine.go`
  strips `os`, `io`, `debug`, file loaders, and even `math.random`, because "scripts
  must be pure compute so replies are cacheable."
- **Text and image pipelines are separate.** The text path uses a narrow
  `onnx.Session` (`input_ids` + `attention_mask` only). Image models need the named
  path. `pipeline/pooling.go` (`ExtractPrePooled`, `ExtractCLS`,
  `MeanPoolAndNormalize`) operates on plain `[]float32` and is reusable.
- **The existing `siglip2` config is text-only** (`text_model_int8.onnx`). The
  `onnx-community/siglip2-base-patch16-224-ONNX` repo also ships
  `onnx/vision_model.onnx` and a fused `model.onnx`.
- **`EMB.MULTI` is multi-model, not multi-text** (alternating `model text` pairs,
  MGET-style nulls). The chosen `EMB.IMGMULTI` mirrors that semantics.
- **The read path has no byte limit.** `redcon` grows its read buffer by doubling
  until a command completes, so a single huge bulk can allocate arbitrarily. This
  matters now that large binaries are first-class.

## Goals / Non-Goals

**Goals:**

- `EMB.IMG` / `EMB.IMGMULTI` over RESP accepting raw image bytes, with the existing
  `BLOB`/`VALUES` reply grammar, batching, caching, and guardrails.
- Preprocessing driven entirely by per-model config (CLIP and SigLIP variants) with
  auto-detection from the ONNX graph and `preprocessor_config.json`.
- Keep the server a pure compute component: no outbound network in the request path.
- Let scripts consume image bytes (preprocess → tensor → `emb.run`) while keeping the
  sandbox network-free and deterministic.

**Non-Goals:**

- Remote URL fetching (rejected — see D2).
- Script HTTP / arbitrary remote fetch (rejected for now; a separate concern).
- Interleaved text+image inputs, video, OCR, reranking, indexing, or a vector store.
- Tensor input (a client-preprocessed `pixel_values` buffer); `EMB.EVAL` + `emb.run`
  already covers that.
- Exact bit-parity with Python/PIL — see the parity risk below.

## Decisions

### D1. Native command, not Lua sugar

`EMB.IMG` is a first-class handler over the named-tensor path, not a preloaded Lua
script. Preprocessing is a host capability regardless (Lua cannot resize), and a
native command reuses the reply grammar, cache, limits, and stats unchanged.

*Alternative:* ship `emb.image.decode` / `emb.image.preprocess` host functions and
let scripts orchestrate. More flexible, but every deployment would need a script,
and reply caching would be gamed by pre-decoded tensors. A native command can back
script functions later without changing this surface.

### D2. Raw bytes only; no URL fetching

`EMB.IMG` accepts only raw encoded image bytes as binary-safe RESP bulks. There is
no URL input, no `data:`/base64, and no scheme sniffing — because the argument is
always bytes, the command name alone disambiguates it from `EMB`. A URL argument
fails decode with an error directing the client to fetch it.

*Alternatives considered and rejected:*

- **URL input with an SSRF policy.** Adds a whole egress surface (scheme/address
  allowlists, rebinding-safe dialing, per-hop redirect validation, timeouts, a fetch
  semaphore, a URL→hash memo) and puts external network latency inside the request
  path. It also doesn't help the stated goal — raw bytes already means "no image to
  text."
- **`data:`/base64.** Pure overhead over a binary-safe protocol: +33% bytes, an
  encode step, and a decode step, for no added capability.

Consequences of the choice, all favorable:

1. **The threat model is unchanged.** `emb` stays a self-hosted, single-binary,
   query-time compute server with no outbound network in the request path.
2. **The cache is optimal.** `sha256(bytes)` needs no fetch and no decode, so a hit
   costs nothing and can never be stale.
3. **Fetching lands where it belongs** — in the client, next to credentials, retries,
   rate limits, and knowledge of which sources are safe.

### D3. Preprocessing is config-driven and auto-detected

A model's `image:` block names the input tensor and carries `size`, `crop`
(none/center), `resample`, `rescale`, `mean`, `std`. Auto-detection fills unset
fields from the ONNX image-input dimensions and `preprocessor_config.json`; explicit
config always wins; an undetectable required field fails load with a named error.

*Alternative:* hardcode "CLIP" and "SigLIP" presets. Rejected — export variants
multiply, and a named preset silently mis-preprocesses a slightly different export.

### D4. Content-addressed cache

Keys are `img:<model>:sha256(bytes)`. This makes changed bytes a different key —
never stale — matching the text cache philosophy, and unlike a URL there is no
network on the hit path. Model-scoped invalidation continues to work because the
key carries the model name.

### D5. `EMB.IMGMULTI` mirrors `EMB.MULTI`

Cross-model, MGET-style per-pair nulls, `max_images` truncation, each pair counted as
a request. Note this is a **transport** convenience: embeddings from different models
are not comparable, so `IMGMULTI` serves fleet routing and heterogeneous corpora, not
cross-model similarity. It is cheap because `EMB.MULTI` already exists.

*Alternative:* make `IMGMULTI` "N images, one model" and drop the cross-model form.
Rejected by product decision; the batch case is already covered by `EMB.IMG`
accepting N images.

### D6. Bounded command size replaces SSRF as the primary hardening

Because images are buffered RESP arguments, the main abuse vector is memory, not
network. The server SHALL enforce a `max_command_bytes` bound and per-image byte and
decoded-pixel caps, rejecting oversized commands before decode/inference. The clean
mechanism is a small addition to the existing `redcon` fork to refuse a bulk whose
declared length exceeds the cap *before* buffering it; a bounded connection reader is
the fallback if the fork is undesirable.

*Trade-off:* the read path currently grows unbounded (`newbuf := make([]byte,
len(rd.buf)*2)`), so without the fork addition the cap can only be enforced after
buffering — which bounds concurrent memory and downstream work but not the initial
allocation. This is a known limitation to close in the fork.

### D7. Reuse the named-session pool; parallel preprocess, single inference

`ModelEntry` gains image resources analogous to `ScriptResources`: a pool of named
sessions plus an immutable preprocessing plan. Preprocessing runs in parallel across
images (CPU-bound, off the session lock); all images for one request are stacked to
`[N,3,H,W]` and inferred in **one** `RunNamed` call. A fixed image size makes
batching trivial (no padding), unlike variable-length text.

### D8. New dependency: `golang.org/x/image`

`image/png|jpeg|gif` are stdlib; resize needs `golang.org/x/image/draw` and WebP
needs `golang.org/x/image/webp`. Pure Go, no CGo, no ONNX Runtime changes. AVIF is
out of scope (no mature pure-Go decoder).

### D9. Scripting integrates image bytes; the sandbox stays network-free

Validation of the Lua path against this feature found three things, which shape the
integration:

1. **Binary already flows into scripts.** `EMB.EVAL`/`EMB.EVSHA` arguments become Go
   strings (byte-preserving) and are delivered as Lua strings, so `KEYS[1]` can carry
   raw image bytes with no encoding or protocol change.
2. **Scripts can already feed `pixel_values`** through `emb.run`, but only as a Lua
   `data` table or `fill`. A 150k-element preprocessed image as a table is slow and
   memory-heavy, so the change adds a packed `bytes` input form (`{shape, bytes,
   dtype}`) — the inverse of `emb.math.float32_bytes` — and has
   `emb.image.preprocess` return exactly that form.
3. **The reply-cache key would bloat.** `script.CacheKey` currently appends the raw
   text to the key, so an image-sized KEYS element would be retained in full for
   every cache entry. The key is changed to digest large payloads while leaving short
   text keys byte-identical, so existing entries stay reachable and hit/miss
   semantics are unchanged.

Because preprocessing is deterministic (identical bytes and config produce identical
tensors), scripted image replies stay cacheable and the pure-compute guarantee holds:
`emb.image.preprocess` adds compute, not network. A script can now do zero-shot
classification (image embedding vs. label embeddings) or cross-modal ranking entirely
inside the sandbox.

## Competitive position and product critique

This is a deliberate challenge to the original idea against the current product and
the market.

### Bytes-only is the correct boundary for *this* product

The differentiation of `emb` is **RESP + Redis-protocol compatibility + self-hosting
in one binary** — no competitor speaks RESP. The URL is a convenience offered by
managed APIs because the fetch happens inside *their* infrastructure, where the
customer is never exposed to SSRF:

| Product | Image input | Who fetches |
|---|---|---|
| Cohere `embed-v4.0` | image + text, multimodal endpoint | Cohere's managed infra |
| Voyage `multimodal-3.5` | `image_url` and `image_base64`, interleaved | Voyage's infra (redirect/content-length/robots limits) |
| CLIP-as-service (Jina) | auto-detects `http(s)`/`data:`/path vs. text | the self-hosted Python server |
| Infinity | serves CLIP/CoLa/CLAP | HTTP inference engine, bytes/base64 |
| Text-Embeddings-Inference | **text only** | n/a |
| Weaviate/Milvus/Qdrant | URL fetch at **ingest** via configured modules | operator-configured, ingest-time |

The pattern is clear: **URL fetching is a managed-SaaS convenience, or an
operator-configured ingest-time module.** It is not something a self-hosted,
query-time, client-controlled compute server should do with arbitrary client input.
`emb` with raw bytes sits exactly where TEI sits for text and where a vector DB's
ingest module sits for URLs — but with the RESP ergonomics none of them have.

### The gap that will actually hurt: the text branch must match

Embedding an image is only useful for search if the **text query** lands in the same
space. Cohere embed-v4, Voyage multimodal, and CLIP-as-service ship a *paired*
text+image model; the current `siglip2` emb config is text-only, and a vision encoder
loaded separately may differ from the text checkpoint in weights, dim, output tensor,
pooling, **and normalization**. If they disagree, text→image retrieval silently
degrades with no error. This is the highest-severity correctness risk, so it is a
spec requirement: `EMB` and `EMB.IMG` on one model must share dimensions, pooling,
and normalization (the output tensor may differ per branch — split
`text_embeds`/`image_embeds` exports are the norm), and a dimension mismatch fails
load.

Related asymmetry: retrieval quality usually depends on a **prompt template** on the
text side ("a photo of a {label}") and sometimes query-vs-document prompts. Voyage
models this with `input_type`. `emb` has no such concept, and native `EMB` cannot
prompt — only scripts can. So a production retrieval setup is `EMB.IMG` **plus** an
`EMB.EVAL` text-prompt script. A future `input_type`/`template` config is the natural
follow-up; it is tracked, not required here.

### Dropping URL input also drops the scripting-fetch ask

The original request had two halves: URL-based image inference, and letting Lua
scripts fetch remote data. Those are the same idea — "let the server get remote
data." If the server should not fetch an image on a client's behalf, it should not
fetch arbitrary data on a script's behalf either, which is strictly more dangerous.
Keeping the sandbox network-free preserves the determinism guarantee that makes
script replies cacheable, and removes the need for load-time impurity detection. If
general remote fetch is ever genuinely needed, it deserves its own change and review.

### Net assessment

Image embedding over RESP with raw bytes is well-positioned and low-risk: it reuses
machinery that already exists, adds no network surface, and is genuinely
differentiated by the protocol. The two things to get right are **dual-encoder
pairing** (correctness) and **command-size bounds** (memory). Both are addressable
in this change.

## Risks / Trade-offs

- **Preprocessing parity drift vs. Python/PIL** → config pinned per model; a golden
  fixture (mirroring `tokenizer/reference_test.go` and `gliner_golden.json`)
  comparing Go preprocessing + a vision encoder against a reference within a
  documented tolerance; document that mixed sources must use the same pipeline.
- **Silent cross-modal degradation** → D5/spec: load-time check that text and image
  share dim/output/pooling/normalization; document prompt-template guidance.
- **Memory blowup from oversized payloads** → D6 command/byte/pixel caps; redcon fork
  addition to refuse declared-oversize bulks before buffering.
- **No URL convenience** → accepted trade-off; clients fetch. If demand appears, the
  sane design is an operator-configured allowlist (the vector-DB ingest pattern),
  not arbitrary per-request URLs, and it should be a separate change.
- **Client binary handling** → the Ruby client must pass `ASCII-8BIT` strings
  unchanged; add a binary round-trip test. Reply decoding is unchanged (same float32
  blob).
- **Script reply-cache key bloat** → D9 digest for large KEYS payloads; short text
  keys unchanged so existing entries remain reachable.
- **Script image tensors through Lua tables** → D9 packed `bytes` input avoids a
  150k-element round-trip and is charged against the existing tensor budget.
- **Cross-model confusion** → document that `EMB.IMGMULTI` results are not
  comparable; it is transport, not semantics.
- **New dependency weight** → `golang.org/x/image` is pure Go and small; no CGo or
  ORT changes.
- **Dynamic/variable image input shapes (e.g. NaFlex)** → out of scope for v1;
  require a fixed size; fail load descriptively if it cannot be determined.

## Migration Plan

Additive and off by default; no existing behavior changes.

1. **Phase 1 — preprocessing + `EMB.IMG` for raw bytes.** `internal/imageproc`,
   the `image:` config, the command, and the golden parity fixture. This alone
   delivers the full goal.
2. **Phase 2 — `EMB.IMGMULTI`** cross-model, reusing the `EMB.MULTI` plumbing.
3. **Phase 3 — scripting integration**: `emb.image.preprocess`/`emb.image.info`, the
   packed `bytes` tensor input, and the bounded reply-cache key, with a runnable
   example script.
4. **Phase 4 — autoconfig, command-size bounds, documentation, and the Ruby client's
   binary image API.**

Rollback is not configuring `image:` blocks; no data migration exists. Image cache
entries age out via LRU.

## Open Questions

- Concrete default caps (`max_command_bytes`, per-image bytes, decoded megapixels) —
  tunable at implementation time; the specs fix the behavior, not the numbers.
- Whether the command-size bound lands in the `redcon` fork or as a bounded
  connection reader.
- WebP support is cheap via `x/image/webp`; AVIF is deferred until a pure-Go decoder
  is viable.
- Whether the Ruby client exposes `image` (bytes) via `Proxy`, and whether it should
  also accept an `IO`/path for convenience — client ergonomics, not server behavior.
- Whether a future `input_type`/`template` model config replaces the need for
  prompt-template scripts — tracked, not required here.
