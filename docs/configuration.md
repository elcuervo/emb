# Configuration

Server settings live in a YAML file (`-config config.yaml`) or inline flags.

- [Full example](#full-example)
- [Model options](#model-options)
- [Image models and preprocessing parity](#image-models-and-preprocessing-parity)
- [Preloading scripts](#preloading-scripts)
- [Batching](#batching)
- [Caching](#caching)
- [Persistent cache snapshots](#persistent-cache-snapshots)

## Full example

```yaml
listen: ":6379"

# password: "hunter2"
# tls_cert: /etc/emb/cert.pem
# tls_key:  /etc/emb/key.pem
# cache: "auto"   # or "1GB", "256MB", "25%". Empty = disabled
# cache_file: /var/lib/emb/cache.embcache
# cache_load: true
# cache_save: 5m
# cache_save_on_shutdown: true
# cache_restore_limit: auto
# cache_restore_reserve: 10%
# cache_save_rate_limit: 100MB/s
# idle_timeout: 15m, max_connections: 100, max_concurrent_requests: 32
# max_command_bytes: 64MB       # max buffered bytes per command (0 = unlimited)
# max_images: 4096              # images per EMB.IMG/EMB.IMGMULTI (0 = unlimited)
# max_image_bytes: 32MB         # per-image byte cap (0 = unlimited)
# max_image_pixels: 33554432    # decoded pixels per image, header-checked (0 = unlimited)

models:
  minilm:
    onnx: ./models/minilm/model.onnx

  # Pre-pooled siglip2 text encoder (int8 used in production). The graph
  # exports its final embedding as "text_embeds" (768-dim); if you configure
  # an output tensor name that is not in the graph, the server falls back to
  # the detected output and logs which name it auto-selected.
  siglip2:
    onnx: ./models/siglip2/text_model_int8.onnx
    tokenizer: ./models/siglip2/tokenizer.json
    output_tensor: text_embeds
    pooling: none
    normalize: true
    dim: 768
    max_length: 64
    pad_output: true   # graph has a fixed [batch, 64] input; pad to max_length

  # Custom e5 export with pooling + output layers baked into the graph (2D
  # pooled output, already normalized), served with no server-side arithmetic:
  # pooling: none + normalize: false is a pure buffer export.
  e5:
    onnx: ./models/e5/model.onnx
    tokenizer: ./models/e5/tokenizer.json
    output_tensor: pooled_sentence_embeddings_debiased_normalized
    pooling: none
    normalize: false

  # WARNING: the stock HuggingFace export intfloat/e5-small-v2 (and similar)
  # outputs a 3D last_hidden_state. It must NOT be configured with
  # pooling: none — that would slice a 3D buffer (correct only at batch=1).
  # Use pooling: mean (auto-detected) or a pre-pooled 2D export instead.

  # Dual-encoder vision export (CLIP/SigLIP): text EMB and image EMB.IMG share
  # one embedding space. The image: block enables EMB.IMG; omitted fields are
  # auto-detected from the ONNX graph and preprocessor_config.json.
  clip:
    onnx: ./models/clip/model.onnx
    tokenizer: ./models/clip/tokenizer.json
    output_tensor: text_embeds        # text branch output
    pooling: none
    normalize: true
    dim: 512
    image:
      input: pixel_values             # image branch input tensor
      output: image_embeds            # image branch output (dim must match dim)
      size: 224
      crop: center                    # none | center
      resample: bicubic               # nearest | bilinear | bicubic | lanczos
      rescale: 0.00392156862745098    # 1/255
      mean: [0.48145466, 0.4578275, 0.40821073]
      std:  [0.26862954, 0.26130258, 0.27577711]
    image_preload: true
```

## Model options

| Field | Default | Description |
|-------|---------|-------------|
| `onnx` | — | Path to ONNX model file |
| `tokenizer` | `<model-dir>/tokenizer.json` | Path to HuggingFace tokenizer JSON |
| `model_repo` | — | HuggingFace repo (auto-downloads ONNX + tokenizer) |
| `dim` | auto-detected | Embedding dimension |
| `max_length` | auto-detected (or 512) | Max token sequence length |
| `pooling` | auto-detected | `mean` (3D output), `cls` (first token) or `none` (2D pre-pooled) |
| `normalize` | `false` | L2-normalize the output |
| `output_tensor` | auto-detected | ONNX output tensor name |
| `preload` | `false` | Load model at startup instead of on first request |
| `pad_output` | `false` | Pad sequences to `max_length` with trailing zeros (compatibility with legacy implementations that don't pass attention mask) |
| `workers` | auto-tuned | Number of worker goroutines |
| `intra_op_threads` | `cores−2` | ONNX intra-op threads per session. Defaults to `cores−2` to reserve cores for request parsing/dispatch; set explicitly to override |
| `scripts` | `[]` | List of file paths to Lua scripts to preload at boot. Relative paths resolve against the config file's directory; absolute paths are used as-is. Invalid scripts (bad syntax, missing file, oversized) fail startup |
| `image` | — | Image preprocessing block (enables `EMB.IMG`); see [Image models](#image-models-and-preprocessing-parity) |
| `image_preload` | `false` | Warm the image named-session pool at startup instead of on first `EMB.IMG` |
| `batching` | `{timeout: 1, max_batch: 32, max_batch_tokens: 16384}` | Smart batching settings. **Enabled by default** (1 ms window) for every model; set `timeout: 0` to use the worker pool. With batching on, `tokenize_workers` defaults to `min(4, cores)` and the token budget auto-applies |

## Image models and preprocessing parity

A model accepts `EMB.IMG` only when it declares an `image:` block. The block's
fields (`input`, `output`, `size`, `crop`, `resample`, `rescale`, `mean`, `std`)
are all optional: explicit configuration always wins, then
`preprocessor_config.json`, then the ONNX graph (a rank-4 image input's name and
static spatial dimensions), then documented defaults. An undeterminable `size`
is a load error naming the field. `crop: center` resizes the shortest edge then
center-crops (CLIP); `crop: none` resizes directly to `size×size` (SigLIP).

**Dual-encoder pairing.** When one model serves both `EMB` (text) and `EMB.IMG`
(image), the two branches must land in the same embedding space. The server
fails model loading when `dim` and the image output tensor's dimension disagree,
so a text-to-image search can never silently degrade to incomparable vectors.
Use a fused/paired export (e.g. `onnx-community/siglip2-base-patch16-224-ONNX`)
rather than mixing an unrelated vision checkpoint with a text model.

**Preprocessing parity caveat.** The Go preprocessing pipeline is within a
documented tolerance of the Python/Pillow reference, not bit-identical (their
resize kernels differ). Embeddings computed by `emb` are therefore not expected
to match a Python pipeline element-for-element. **Do not mix pipelines for one
index:** if documents were indexed with Python preprocessing, do not query with
`emb` (or vice versa) — pick one and use it for both sides. Clients that need
exact reference parity should preprocess with their own pipeline and send the
tensor through `EMB.EVAL` + `emb.run`.

## Preloading scripts

Scripts normally enter the cache via `EMB.SCRIPT LOAD`, which costs a client
round-trip and a server-side compile on first use. For deployments with known
scripts, declare them in model config and the server reads, validates, and
precompiles them at boot so `EMB.EVSHA` works immediately:

```yaml
models:
  minilm:
    model_repo: Xenova/all-MiniLM-L6-v2
    scripts:
      - scripts/classify.lua      # relative → config file's directory
      - /etc/emb/normalize.lua    # absolute, used as-is
```

A bad script (missing file, invalid Lua, oversized) is **fatal at boot** — the
server refuses to start half-configured, matching existing model validation
semantics. Preloaded scripts are indistinguishable from dynamically loaded
ones: `EMB.SCRIPT EXISTS` reports `1`, `EMB.EVSHA` replies immediately, and
`EMB.SCRIPT FLUSH` drops them like any other cache entry.

## Batching

Batching is **on by default** for every model, so no config is needed for the
performance path: a window coalesces concurrent requests (including `EMB.MULTI`
pairs) into shared ONNX runs, a token budget bounds each run, and dedicated
tokenizer workers hide tokenization behind inference. `timeout: 0` opts out.
`EMB.MULTI` processes pairs with bounded concurrency (≤ the machine's `GOMAXPROCS`),
so request storms can't spawn unbounded goroutines that starve inference.

## Caching

Embeddings are cached by `model:text` key in an in-process LRU, so repeated
texts skip ONNX inference entirely. Configure via the `cache:` YAML key or the
`-cache` CLI flag:

| Value | Behavior |
|-------|----------|
| *(empty)* | Cache disabled (default) |
| `auto` | ~13% of total RAM: 20% of memory left after a 10% safety margin and a 25% model reserve, floored at 64MB. No fixed byte cap — scales with the machine |
| `1GB`, `256MB`, … | Explicit byte budget (`docker/go-units` sizes) |
| `25%` | Percentage of total system RAM (explicit operator choice — no safety margins applied) |

```bash
./bin/emb -config config.yaml -cache auto   # ~13% of RAM on this machine
./bin/emb -config config.yaml -cache 25%    # a quarter of RAM
./bin/emb -config config.yaml -cache 2GB    # fixed budget
```

Invalid sizes or percentages (e.g. `150%`, `abc`) fail startup with a clear
error. Live cache stats — hits, misses, hit rate, evictions, entries, and byte
usage — are visible per model via `EMB.INFO <model>` and globally via `INFO` and
`EMB.STATS`. See [BENCHMARK.md](../BENCHMARK.md) → *Cache* for hit-rate
measurements.

## Persistent cache snapshots

Snapshots optionally preserve the in-process LRU across restarts. They are
inspired by Redis RDB operationally, but are a small emb-specific streaming
format and are **not RDB-compatible**. Persistence is completely dormant when
`cache_file` is empty: no coordinator, timer, fingerprinting, serialization,
filesystem work, or cache hot-path checks are created.

| Setting / CLI flag | Default | Live update | Meaning |
|---|---:|---:|---|
| `cache_file` / `-cache-file` | empty | yes | Snapshot path; empty disables persistence |
| `cache_load` / `-cache-load` | `true` | restart | Restore at startup when the file exists |
| `cache_save` / `-cache-save` | empty | yes | Positive automatic-save interval; empty disables periodic saves |
| `cache_save_on_shutdown` / `-cache-save-on-shutdown` | `true` | yes | Save a dirty cache after inference drains and before model close |
| `cache_restore_limit` / `-cache-restore-limit` | `auto` | restart | Maximum restore memory as `auto`, bytes, or percentage of host RAM |
| `cache_restore_reserve` / `-cache-restore-reserve` | `10%` | restart | Host memory kept free during restore |
| `cache_save_rate_limit` / `-cache-save-rate-limit` | `0` | yes | Output rate such as `100MB/s`; `0` is unlimited |

Startup restore streams into an unpublished staging cache. Its effective
ceiling is the smallest of the configured cache budget, `cache_restore_limit`,
and sampled host headroom (`total RAM - current RSS - reserve`). Compatible
entries are admitted MRU-first; checksum failure leaves the live cache empty.
Model/tokenizer fingerprints are streamed once and cached, so periodic saves
do not repeatedly read model artifacts.

Automatic and manual saves briefly capture immutable entry descriptors under
the cache mutex, then encode, checksum, throttle, sync, and rename in a
background goroutine. Inference and all database commands continue during
that work. Only one save runs at a time; clean timer ticks are coalesced and
`EMB.SAVE` reports an overlap error. Files are written through an owner-only
`0600` temporary file and atomically renamed, preserving the prior snapshot
until the replacement is complete.

Snapshot files contain original input text and embedding bytes. Protect or
encrypt their storage as the source data requires. They are a disposable
warm-start optimization, not a durable database or backup; deleting a bad or
incompatible file is always safe.

Live changes to `cache_file` and the `EMB.SAVE` command require either a
configured `password` or a loopback-only `listen` address. The default
listener (`:6379`) binds every interface, so with no `password` set the server
rejects those commands with an explicit error rather than letting an
unauthenticated client point snapshot writes at an arbitrary path. Bind
`localhost` for a password-free deployment, or set `password` (and `AUTH`)
when serving non-loopback clients.
