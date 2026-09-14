# emb

A simple yet powerful text embeddings generator.

[![GitHub Release](https://img.shields.io/github/v/release/elcuervo/emb?logo=github&color=blue)](https://github.com/elcuervo/emb/releases)
[![Docker Hub](https://img.shields.io/docker/v/elcuervo/emb?logo=docker&color=blue&label=docker)](https://hub.docker.com/r/elcuervo/emb)
[![emb gem](https://img.shields.io/gem/v/emb?logo=rubygems&color=red&label=emb)](https://rubygems.org/gems/emb)
[![emb-server gem](https://img.shields.io/gem/v/emb-server?logo=rubygems&color=red&label=emb-server)](https://rubygems.org/gems/emb-server)

<a href="https://emb.is">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="assets/emb-wordmark-dark.svg">
    <source media="(prefers-color-scheme: light)" srcset="assets/emb-wordmark-light.svg">
    <img alt="emb — bytes in. vectors out." src="assets/emb-wordmark-light.svg" width="260" height="115">
  </picture>
</a>

`emb` is a text-embeddings server speaking the Redis protocol. Every Redis
client: `redis-cli`, `redis-py`, `redis-rb`, … — can call it without special
libraries, and embeddings come back as raw float32 bytes by default:

```bash
redis-cli EMB minilm "hello world"
# → \x7c\x8e\x80\xbd...   (384 float32s × 4 bytes)

# RESP3 clients can ask for a self-describing decimal reply instead:
redis-cli -3 EMB minilm VALUES "hello world"
# → dtype FLOAT / shape [1 384] / values [-0.1974, 0.1776, ...]
```

## Contents

- [Features](#features)
- [Install](#install)
- [Quick start](#quick-start)
- [Commands](#commands)
- [Custom scripts](#custom-scripts)
- [Configuration](#configuration)
- [Operations](#operations)
- [Monitoring: emb-top](#monitoring-emb-top)
- [Clients](#clients)
- [Development](#development)

## Features

- **Redis protocol** — drop-in for any Redis client; RESP2 by default with
  opt-in RESP3 (`HELLO 3`). Embeddings default to compact little-endian float32
  bytes, with a `VALUES` format that returns self-describing decimal replies
  (typed RESP3 doubles or RESP2 decimal bulks) for non-binary clients.
- **ONNX Runtime** — fast CPU/GPU inference via CGo bindings, with optional
  int8 weight quantization.
- **HuggingFace integration** — auto-download models and auto-detect dim,
  max_length, output tensor, and pooling strategy from the ONNX graph + `config.json`.
- **Smart batching** — a 1 ms window coalesces concurrent requests into shared
  ONNX runs, with a token budget and async tokenization (on by default).
- **Embeddings cache** — in-process LRU with per-model stats; sized in bytes,
  percentages, or `auto`.
- **Multi-model queries** — `EMB.MULTI` calls different models in one command
  (MGET-style partial failures).
- **Image embeddings** — `EMB.IMG` / `EMB.IMGMULTI` accept raw JPEG/PNG/GIF/WebP
  **bytes** over RESP (binary-safe, no base64 or URL), decode and preprocess them
  server-side to the model's `pixel_values` tensor, and return embeddings in the
  same `BLOB`/`VALUES` grammar. Content-addressed image caching, command/image
  size caps, and Lua access via `emb.image.preprocess` are included. The server
  never fetches URLs.
- **Ops-ready** — Redis-style `INFO` and `CONFIG`, health checks
  (`EMB.READY`), connection lifecycle knobs, and full server stats.

## Install

```bash
curl -fsSL https://github.com/elcuervo/emb/raw/main/install.sh | sh
```

Installs to `/usr/local/bin`. Set `EMB_INSTALL_DIR` to change the target:

```bash
curl -fsSL https://github.com/elcuervo/emb/raw/main/install.sh | EMB_INSTALL_DIR=~/.local/bin sh
```

**Platforms:** macOS (Apple Silicon), Linux (amd64, arm64).

Or install the [`emb-server`](https://rubygems.org/gems/emb-server) gem and run
`emb` directly.

## Quick start

### One-liner (no config file)

```bash
# Auto-downloads a model from HuggingFace and starts the server
emb -model-repo Xenova/all-MiniLM-L6-v2

# With password authentication
emb -model-repo Xenova/all-MiniLM-L6-v2 -password "hunter2"

# In another terminal:
redis-cli EMB model "hello world"
```

### Two models inline

```bash
emb \
  -model minilm -model-onnx ./models/minilm/model.onnx -model-tokenizer ./models/minilm/tokenizer.json \
  -model bge   -model-repo Xenova/bge-small-en-v1.5

redis-cli EMB.MULTI minilm "hello" bge "world"
```

### Local development (with config file)

```bash
just download-model   # Download a model from HuggingFace
just dev              # Build and start the server

# In another terminal:
redis-cli EMB minilm "hello world"
```

## Commands

| Command | Description |
|---------|-------------|
| `EMB <model> [BLOB\|VALUES] <text> [text...]` | Embed one or more texts. Default `BLOB`: single text → bulk string, multiple → array of bulk strings (float32 bytes). `VALUES`: `dtype`/`shape`/`values` envelope with decimal values |
| `EMB.MULTI [BLOB\|VALUES] <model> <text> [<model> <text>...]` | Embed texts across different models in one call; per-pair `VALUES` envelopes (with `model`) or null on failure |
| `EMB.IMG <model> [BLOB\|VALUES] <bytes> [<bytes>...]` | Embed one or more images from raw JPEG/PNG/GIF/WebP bytes (each a binary-safe bulk). `BLOB`: single image → bulk, multiple → array with nulls for failed/truncated slots; `VALUES`: one `[m, dim]` envelope over processed images. URLs are rejected |
| `EMB.IMGMULTI [BLOB\|VALUES] <model> <bytes> [<model> <bytes>...]` | Embed images across different models in one call; MGET-style per-pair nulls, per-pair `VALUES` envelopes (with `model`) |
| `EMB.MODELS` | List loaded models with dimensions and status |
| `EMB.INFO <model>` | Model details: dim, workers, requests served, avg latency, live cache stats |
| `EMB.STATS` | Server statistics: uptime, total requests, live connections, active requests, per-model breakdown, **mem (RSS MB), cpu user/sys usec, goroutines** |
| `MONITOR [seq] [limit]` | Recent completed-request events (`seq`, timestamp µs, model, texts, latency µs, error) from a bounded ring. Incremental (`seq`) fetch; **no text payloads** |
| `EMB.READY` | Health check: `+OK` (ready), `-ERR <reason>` (loading, draining, no models) |
| `EMB.EVAL <model> <script> <numtexts> <text...> <arg...>` | Evaluate a Lua script once against a model (KEYS=texts, ARGV=args) |
| `EMB.EVSHA <model> <sha> <numtexts> <text...> <arg...>` | Evaluate a cached script by SHA (see `EMB.SCRIPT LOAD`) |
| `EMB.SCRIPT LOAD <model> <script>` | Compile, cache, and return the script's SHA1 |
| `EMB.SCRIPT EXISTS <model> <sha...>` | Which scripts are cached (1/0 per SHA) |
| `EMB.SCRIPT FLUSH [<model>]` | Clear cached scripts (all models when omitted) |
| `EMB.CACHE.FLUSH [model]` | Remove all cached embeddings, or only entries for one configured model; returns the removed count |
| `EMB.SAVE` | Accept an asynchronous cache snapshot; poll `EMB.STATS` or `INFO cache` for completion/failure |
| `EMB.HELP` | Command reference (includes the full script surface) |
| `INFO [section...]` | Redis-style INFO: `server`, `cache`, `keyspace`, `stats`, `memory`, `cpu`, `clients` |
| `CONFIG GET [glob]` / `CONFIG SET` | Read or live-tune runtime settings (see [Operations](#operations)) |
| `AUTH <password>` | Authenticate the connection (required if `password` is set) |
| `HELLO [2\|3]` | Negotiate the RESP protocol version for the connection (default 2); bare `HELLO` reports the current version |
| `PING` | PONG |

### EMB.MULTI

`EMB.MULTI` embeds texts against different models in a single round trip and
answers with MGET-style partial failures: each reply is the vector for its
model, and a failed pair yields an error in its slot without failing the rest.

```
redis-cli EMB.MULTI minilm "hello" siglip2 "a photo of a cat"
1) \x7c\x8e\x80\xbd...   (minilm, 384 floats)
2) \x4a\x9f\x31\xc2...   (siglip2, 768 floats)
```

### Image embeddings: EMB.IMG

`EMB.IMG` embeds images directly from their **encoded file bytes** — no base64,
no URL, no client-side preprocessing. RESP bulk strings are binary-safe, so a
client just sends the file:

```bash
# redis-cli -x reads the last argument from stdin:
cat cat.jpg | redis-cli -x EMB.IMG siglip2

# multiple images → one embedding slot each (null on a failed/truncated slot):
redis-cli EMB.IMG siglip2 <cat.jpg bytes> <dog.png bytes>

# cross-model, MGET-style per-pair nulls:
redis-cli EMB.IMGMULTI clip <cat.jpg bytes> siglip2 <dog.png bytes>
```

The server decodes the image (PNG/JPEG/GIF/WebP), resizes/crops, rescales, and
normalizes it to the model's `pixel_values` tensor (`[1, 3, H, W]` float32 RGB),
then runs **one batched inference** for all images in the command. The reply is
the same `BLOB`/`VALUES` grammar as text (`EMB.IMG` takes the keyword at
position 2, `EMB.IMGMULTI` at position 1).

**Bytes only — the server never fetches URLs.** An `http(s):` or `data:` URI
argument is rejected with an error telling the client to fetch the image and
send its bytes. This keeps the request path network-free (no SSRF surface) and
makes image caching optimal: entries are keyed `img:<model>:sha256(bytes)`, so a
changed image is always a different key and a hit costs no decode or network.

Image limits are configurable and enforced before decode/inference:
`max_images` (default 4096, `0` = unlimited), `max_image_bytes` (default 32 MiB),
`max_image_pixels` (default 33.5 MP, checked from the header), and the
command-wide `max_command_bytes` (default 64 MiB). A single bad or oversized
image fails only its own slot; overflow images past `max_images` are truncated
to null slots without being decoded. See [Configuration](#configuration).

### Reply formats: BLOB and VALUES

`EMB` and `EMB.MULTI` accept an optional leading reply-format keyword,
mirroring RedisAI's `AI.TENSORGET <key> [META] [BLOB|VALUES]` (`EMB.IMG` takes it
at position 2, right after the model; `EMB.IMGMULTI` at position 1):

- **`BLOB`** (default) — the compact binary wire: raw little-endian float32
  bytes as bulk string(s). Fastest, and byte-identical to prior emb versions.
- **`VALUES`** — a self-describing envelope: `dtype` (`FLOAT`), `shape`
  `[m, dim]`, and a flat row-major `values` array of the embeddings as decimal
  floats. Handy for clients, tooling, and debugging that cannot decode raw
  float32 bytes.

```bash
redis-cli EMB minilm VALUES "hello world"
1) "dtype"
2) "FLOAT"
3) "shape"
4) 1) (integer) 1
   2) (integer) 384
5) "values"
6) 1) "-0.19744610786437988"
   2) "0.17766517400741577"
   ...
```

`EMB.MULTI ... VALUES` returns one envelope per pair (including a `model`
key, since per-pair dimensions differ across models), with nulls for failed
pairs. The keyword is detected only at its fixed position right after the
model name (or before the pairs) — it is never scanned from the text tail — so
trailing text that happens to read `BLOB` or `VALUES` embeds normally. The one
collision is a **first** text in a multi-text call: `EMB m VALUES hello`
treats `VALUES` as the keyword, so to embed the literal word first write it
twice — `EMB m VALUES VALUES hello` (or reorder the texts). As a safety
measure, `BLOB` and `VALUES` are reserved and cannot be used as model names.

### RESP3 and protocol negotiation

Connections speak RESP2 by default. A client can upgrade a connection to
RESP3 with `HELLO 3` (and back with `HELLO 2`); bare `HELLO` reports the
current version. Under RESP3:

- `EMB ... VALUES` values are typed RESP3 doubles (`,<decimal>`) instead of
  decimal bulk strings.
- `EMB.INFO`, `EMB.STATS`, `EMB.MODELS`, and `CONFIG GET` reply with maps
  instead of flat key/value arrays; nulls encode as `_` instead of `$-1`.
- `INFO` stays a bulk string in both protocols (as in real Redis), and errors
  stay simple errors.

The Ruby client (`gems/emb`) keeps RESP2 and the binary `BLOB` default; pass
`format: :values` to opt into decimal replies. See [Clients](#clients).

## Custom scripts

### The model, as a function

Think of the plain embed command as a fixed pipeline:

```text
model(input) -> output          # EMB <model> <text>
```

tokenize → infer → pool → normalize → reply. **Scripts** replace the outer
edges of that pipeline with your own code around the same model call:

```text
model(fn(input)) -> output      # EMB.EVAL / EMB.EVSHA
```

`fn` is a sandboxed Lua function on the server that does your preprocessing
(custom tokenization, constant inputs, prompt framing) and postprocessing
(argmax, softmax, byte packing) — then replies through the exact same RESP
grammar as the embed path. Scripts are cached by SHA1 per model, so hot
requests are one `EMB.EVSHA` round trip with no server-side re-compile.

### The script surface

Scripts read texts from `KEYS` and arguments from `ARGV`
(`EMB.EVSHA <model> <sha> <numtexts> <text...> <arg...>`). A multi-text call
runs **one** evaluation with `KEYS` = all texts and requires one returned
value per text. The whitelisted host blocks:

| Block | Purpose |
|-------|---------|
| `emb.run(spec [, opts])` | Named-tensor inference → `{name = {shape, data, dtype}}` per output; `opts = {bytes = true, outputs = {"name", ...}}` |
| `emb.run_batch({item, ...} [, opts])` | One model call for N items (padded into a single session run), same `opts` |
| `emb.embed(text \| {texts...} [, {bytes = true}])` | **Pooled, normalized embedding(s)** through the server's embedding path: shares the batcher, the `model:text` cache, and the ORT sessions with `EMB` |
| `emb.image.embed(bytes \| {bytes...} [, {bytes = true}])` | Pooled image embedding(s) from the model's image branch, in the same space as `emb.embed` (URLs rejected) |
| `emb.similarity(a, b [, metric])` | **Higher = more similar**: `cosine` (default), `dot` |
| `emb.distance(a, b [, metric])` | **Lower = closer**: `l2` (default), `l2sq`, `cosine` (= `1 − cosine`) |
| `emb.tokenize.encode(text, max_len)` | The model tokenizer's own pipeline → `{ids, mask, offsets}` |
| `emb.tokenize.encode_pair(a, b, max_len)` | BERT-family pair framing → `{ids, mask, offsets, sep}` |
| `emb.tokenize.words(text)` | Generic word split (BertPreTokenizer rules, byte offsets) |
| `emb.tokenize.pretokenized(words, max_len)` | Encode an already-split word list → `{ids, word_ids}` |
| `emb.math.{sigmoid, softmax, argmax}` | Post-processing primitives (accept a number, an array, **or** packed bytes); scalar `sigmoid(x)`/`softmax(x) == 1`/`argmax(x) == (1, x)` |
| `emb.math.{dot, cosine, l2, norm}` | Vector reductions over arrays or packed bytes |
| `emb.math.mean_pool(hidden, shape, mask)` / `emb.math.cls(hidden, shape)` | Pooled + L2-normalized vectors per batch row, host-side |
| `emb.math.{topk, gather, slice, scale, add}` | Selection/arithmetic without interpreted loops (scalar arguments must be integers) |
| `emb.math.float32_bytes(vals)` | Pack numbers into ONE little-endian float32 bulk (`unpack('e*')`) |
| `emb.image.preprocess(bytes)` | Decode and preprocess raw image bytes with the model's `image:` plan → `{shape, bytes, dtype, input}` ready for `emb.run` |
| `emb.image.info()` | The model's configured image preprocessing parameters (`input`, `size`, `crop`, `resample`, `rescale`, `mean`, `std`) |
| `emb.API_VERSION` | Host-surface version string, for scripts that must detect an older server |
| `json.{encode, decode, null}` | Structured replies / parsing; `json.null` is the unique null sentinel (arrays round-trip) |

`emb.embed` and `emb.image.embed` are available only for models that configure an
embedding (`dim` + `pooling != none`) or image branch respectively; on any other
model the function is absent, exactly like `emb.image` on a text-only model.
`emb.image.embed` runs through the same image session pool and content-addressed
cache as `EMB.IMG`, so an image embedded by either path is a cache hit for the
other. Image sessions open lazily on first `emb.image.embed`; `emb.image.info`
and `emb.image.preprocess` need only the preprocessing plan, so a script that
never touches the image surface allocates no image resources.

Input specs are `{shape = {...}, data = {...}, dtype?}` — or
`{shape = {...}, fill = n, dtype?}` to build a **constant tensor host-side**,
or `{shape = {...}, bytes = <string>, dtype = "f32"|"i64"}` to feed packed
little-endian elements with no per-element Lua table (the inverse of
`emb.math.float32_bytes`; `dtype` is required and the byte length must match the
shape exactly). `data`, `fill`, and `bytes` are mutually exclusive; an explicit
`dtype` (`"f32"`/`"i64"`) wins, and a fractional `fill` infers `f32`:

```lua
-- fused-CLIP text branch: a zeroed 1×3×224×224 pixel_values built by the
-- host — a Lua zeros table here would be 150,528 elements per request.
emb.run({
  input_ids    = { shape = {1, #enc.ids}, data = enc.ids },
  pixel_values = { shape = {1, 3, 224, 224}, fill = 0, dtype = "f32" },
})
```

Image bytes become a runnable tensor the same way, without a 150k-element
conversion: `emb.image.preprocess` returns a packed spec you pass straight to
`emb.run` (see [`examples/scripts/snippets/image_zeroshot.lua`](examples/scripts/snippets/image_zeroshot.lua)):

```lua
local spec = emb.image.preprocess(KEYS[1])   -- KEYS[1] = raw image bytes
local out  = emb.run({ [spec.input] = { shape = spec.shape, bytes = spec.bytes, dtype = spec.dtype } })
```

Replies convert through the standard grammar: Lua string → bulk, list → array,
string-keyed table → hash (flat field/value pairs), `{err = "..."}` → error
reply. `EMB.HELP` lists the full surface.

### JSON values and math semantics

`json.encode` / `json.decode` round-trip any JSON value. A Lua table cannot
hold `nil`, so a decoded `null` — in an object value **or** an array element —
is stored as the unique `json.null` sentinel, and encoding that sentinel
reproduces `null`:

```lua
json.encode(json.decode('[1,null,3]'))   -- [1,null,3]
json.encode({1, json.null, 3})           -- [1,null,3]
```

The `emb.math` helpers share one rule for scalars, empties, and argument types:

- `sigmoid`, `softmax`, and `argmax` accept a single number as well as an array
  or packed buffer. The scalar forms are the degenerate ones: `sigmoid(x)`,
  `softmax(x) == 1`, and `argmax(x) == (1, x)`.
- An operand with **zero elements** is valid exactly where the operation has a
  defined empty result: element-wise maps (`sigmoid`, `scale`, `add`) and
  selections (`topk`, `gather`, `slice`) return `{}`; linear reductions (`dot`,
  `l2`, `norm`) return `0`; and `softmax`, `argmax`, `cosine`, and
  `float32_bytes` error.
- Scalar arguments must be integers: a fractional `k`, `offset`, `length`, or
  index is an error, not a silent truncation. `shape` dimensions are validated
  the same way.

Every `emb.run` / `emb.run_batch` output carries its `dtype` in both forms
(`{shape, data, dtype}` and the packed `{shape, bytes, dtype}`), so any output
can be fed straight back as an input spec without the server re-inferring the
dtype from element values.

### Packed and selective outputs

`emb.run` and `emb.run_batch` take an options table as their final argument:

- `{bytes = true}` returns each output as `{shape = {...}, bytes = <string>, dtype = "f32"|"i64"}`
  — one Lua string of little-endian raw elements, the exact inverse of the
  `bytes` input form. Nothing is materialized element-by-element, which is what
  keeps large graph outputs cheap. Without the option each output is the
  equivalent array form `{shape = {...}, data = {...}, dtype = ...}`.
- `{outputs = {"logits"}}` materializes only the named outputs; an unknown name
  is an error listing what the graph produces.

Both forms are charged against the same per-evaluation tensor-element budget.

### Similarity and distance

`emb.similarity` and `emb.distance` are the two polarities, kept separate so
neither is ambiguous:

| Call | Returns | Metrics |
|------|---------|---------|
| `emb.similarity(a, b [, metric])` | **higher = more similar** | `cosine` (default), `dot` |
| `emb.distance(a, b [, metric])` | **lower = closer** | `l2` (default), `l2sq`, `cosine` (= `1 − cos`) |

Both operands may be a Lua array of numbers **or** a packed float32 string (the
form `emb.embed(..., {bytes = true})` and `emb.math.float32_bytes` produce), and
the two can be mixed. `emb.similarity(a, b, "cosine") + emb.distance(a, b, "cosine") == 1`.
A zero-magnitude operand scores `0` for cosine rather than erroring.

```lua
-- text ↔ text, one round trip, using the shared embedding cache
local v = emb.embed({ KEYS[1], ARGV[1] }, { bytes = true })
return emb.similarity(v[1], v[2])
```

### Production scripting

Scripts are a supported inference path, not just a demo surface. What that
requires in practice:

- **Determinism and caching.** Scripts are pure compute: `os`, `io`,
  `require`/loaders, coroutines and `math.random*` are stripped, so identical
  inputs always produce identical replies. Replies are cached per
  `(model, script SHA1, args, text)`, so a script's output must depend only on
  those. For a **pairwise** operation (similarity, rerank, cross-encoder), put
  one operand in `KEYS` and the other in `ARGV` — the ARGV hash is part of the
  key, so distinct pairs stay distinct cache entries.
- **Embeddings vs raw tensors.** Use `emb.embed` / `emb.image.embed` when the
  model has an embedding configuration: they run the same pooling and
  normalization as `EMB`, share its batcher and `model:text` cache, and never
  open a second model. Use `emb.run` / `emb.run_batch` (with `{bytes = true}`)
  for graphs whose outputs are not an embedding — logits, spans, scores — and
  reduce them with `emb.math.*` rather than interpreted Lua loops.
- **Packed buffers.** Reducing a packed output with `emb.math.mean_pool`,
  `emb.math.gather`, `emb.math.sigmoid`, … keeps the work in the host. Indexing
  a large tensor element-by-element in Lua is the single biggest avoidable cost
  in a script.
- **Limits.** Each evaluation is bounded by a wall-clock deadline, a call-stack
  depth limit, a script-size cap, and a per-evaluation tensor-element budget
  (inputs and outputs both count). Exceeding a bound fails only that request.
  Scripted commands count toward `max_concurrent_requests`.
- **Memory and parallelism.** A scripted model loads its named-tensor sessions
  and tokenizer **lazily**: a script that returns a constant, or that only uses
  `emb.embed`, opens none. When a script calls `emb.run`, `script_workers`
  decides how many named-tensor sessions exist:

  | `script_workers` | Sessions opened | Trade-off |
  |---|---|---|
  | unset / `0` | one per session the embedding pool actually holds (`1` under the batching default, `workers` otherwise) | matches the embedding path's memory and concurrency; the safe default |
  | explicit `N` | exactly `N` | each session is a separate model instance (~model size in RSS); raises scripted parallelism for extraction workloads |

  An explicit value is honoured verbatim — it is an operator override, never
  clamped. The tokenizer is shared with the embedding pool (one per model).
- **Observability.** Scripted evaluations appear in `MONITOR` and are counted
  separately in `EMB.STATS` (`script_requests`, `script_errors`,
  `script_avg_latency_us`, `per_model_scripts`); `EMB.INFO <model>` reports the
  model's open script session count.
- **Version.** `emb.API_VERSION` identifies the host surface; a script that
  needs a newer function can detect an older server and report its own error.

### Example 1 — vector embeddings (`model(input) → output`)

[`examples/scripts/snippets/siglip2.lua`](examples/scripts/snippets/siglip2.lua) re-implements
the embed path on a fused CLIP export: the image branch is fed a constant zero
tensor (`fill`), the text embedding is L2-normalized, and the reply is **one
3 KB bulk** byte-identical to a direct embed (768 float32s, `unpack('e*')`)
instead of 768 separate RESP bulks:

```lua
local enc = emb.tokenize.encode(KEYS[1], 256)
local out = emb.run({
  input_ids    = { shape = {1, #enc.ids}, data = enc.ids },
  pixel_values = { shape = {1, 3, 224, 224}, fill = 0, dtype = "f32" },
})
local vec = out.text_embeds.data
if ARGV[1] == "normalize" then
  local norm = 0
  for i = 1, #vec do norm = norm + vec[i] * vec[i] end
  norm = math.sqrt(norm)
  if norm > 0 then
    for i = 1, #vec do vec[i] = vec[i] / norm end
  end
end
return emb.math.float32_bytes(vec)
```

```bash
SHA=$(redis-cli EMB.SCRIPT LOAD siglip2 "$(cat examples/scripts/snippets/siglip2.lua)")
redis-cli EMB.EVSHA siglip2 "$SHA" 1 "a photo of a cat" normalize
# -> one 3072-byte bulk string (768 little-endian float32s)
```

### Example 2 — custom classification (`model(fn(input)) → output`)

[`examples/scripts/snippets/sst2.lua`](examples/scripts/snippets/sst2.lua) turns a raw-logits
text model into a labeled classifier with the building blocks: encode with the
model's own tokenizer, run once, softmax–argmax against labels passed as
`ARGV`:

```lua
local labels = {}
for i = 1, #ARGV do labels[i] = ARGV[i] end

local enc = emb.tokenize.encode(KEYS[1], 512)
local out = emb.run({
  input_ids      = { shape = {1, #enc.ids}, data = enc.ids },
  attention_mask = { shape = {1, #enc.ids}, data = enc.mask },
})
local probs = emb.math.softmax(out.logits.data)
local idx, score = emb.math.argmax(probs)
return { label = labels[idx], confidence = score, scores = probs }
```

```bash
redis-cli EMB.EVSHA sst2 "$SHA" 1 "this film is great" NEGATIVE POSITIVE
# -> hash: {label = "POSITIVE", confidence = 0.99, scores = [...]}
```

Example scripts are split into **maintained reference** implementations
([`examples/scripts/reference/`](examples/scripts/reference/), exercised by CI
against a downloaded model) and **illustrative snippets**
([`examples/scripts/snippets/`](examples/scripts/snippets/)). The GLiNER2 span
extractor ([`reference/gliner2.lua`](examples/scripts/reference/gliner2.lua))
is the maintained reference: it reads its logits tensor in packed form and
decodes with `emb.math.gather` + `emb.math.sigmoid`. The snippets cover
extractive QA (`qa.lua` — pair encode + constrained span search over offsets),
reranking (`rerank.lua` — per-document batched sigmoid scores), sequence
classification (`sst2.lua`), and cross-modal zero-shot
(`image_zeroshot.lua`). See
[`examples/scripts/README.md`](examples/scripts/README.md) for the full
table.

For the whole loop rather than one construct, see
[`examples/kitchensink/`](examples/kitchensink/): a runnable application that
embeds a corpus with an `emb` instance, stores the vectors in a Redis vector
set, and answers ranked queries — `emb` computing, Redis storing and searching,
two servers and one protocol between them.

### Writing your own script

1. **Know your graph.** Input tensor names/ranks/dtypes and the output tensor
   come from your ONNX export; the server logs the output name it
   auto-detects at model load.
2. **Preprocess** with the `emb.tokenize.*` blocks, then build each input as
   `{shape, data}` (element-wise) or `{shape, fill}` (constant, host-side).
3. **Run the model once** with `emb.run` (or `emb.run_batch` for N items in
   one session call) and **postprocess** with `emb.math.*` until the reply
   matches the grammar you want your clients to consume.
4. **Load and call** — the SHA1 is per model, so re-loading after an edit is
   a new SHA:

```bash
SHA=$(redis-cli EMB.SCRIPT LOAD minilm "$(cat my_script.lua)")
redis-cli EMB.EVSHA minilm "$SHA" 1 "hello world" normalized
```

or straight from Ruby:

```ruby
sha = client.script.load(:minilm, source)
embedding = client.evalsha(:minilm, sha, ["hello world"], ["normalized"], decode: :f32)
# -> [0.0123, -0.0456, ...]   decode: :f32 unpacks float32_bytes replies
```

Sandbox notes: scripts are **pure compute** — `os`, `io`, `require`/loaders,
coroutines, and `math.random*` are stripped, so identical inputs always
produce identical replies (which is what makes reply caching sound).

Resource notes: a scripted model loads its named-tensor sessions and tokenizer
**lazily** — a script that returns a constant, or that only calls `emb.embed`,
never opens them. When a script calls `emb.run`, `script_workers` sets the
number of named-tensor sessions: unset/`0` opens one per session the embedding
pool actually holds (`1` under the batching default, `workers` otherwise), while
an explicit value is honoured verbatim as an operator override — each session is
a separate model instance, so raising it trades memory for scripted
parallelism. It is never clamped. The tokenizer is shared with the embedding
pool (one per model). Scripted traffic is reported separately under
`script_requests` / `script_errors` / `script_avg_latency_us` and
`per_model_scripts` in `EMB.STATS`, appears in `MONITOR`, and each model's
script and image session counts are visible in `EMB.INFO`
(`script_sessions`, `image_sessions`).

## Configuration

Server settings live in a YAML file (`-config config.yaml`) or inline flags:

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

### Model options

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

### Image models and preprocessing parity

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

### Preloading scripts

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

### Batching

Batching is **on by default** for every model, so no config is needed for the
performance path: a window coalesces concurrent requests (including `EMB.MULTI`
pairs) into shared ONNX runs, a token budget bounds each run, and dedicated
tokenizer workers hide tokenization behind inference. `timeout: 0` opts out.
`EMB.MULTI` processes pairs with bounded concurrency (≤ the machine's `GOMAXPROCS`),
so request storms can't spawn unbounded goroutines that starve inference.

### Caching

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
usage — are visible per model via `EMB.INFO <model>` and globally via
[`INFO`](#operations) and `EMB.STATS`. See `BENCHMARK.md` → *Cache* for
hit-rate measurements.

### Persistent cache snapshots

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

## Operations

### Health checks

`EMB.READY` returns `+OK` when the server is ready to serve traffic, or `-ERR`
with a reason (`loading`, `draining`, `no models`). Point your load balancer's
TCP health check at it, or use the client's `ready?`/`ready` helpers:

```bash
redis-cli EMB.READY
# → OK
```

```ruby
Emb.ready?  # => true
Emb.ready   # => "ready"
```

### Connection lifecycle

Three knobs bound the server's connection and request surface. `idle_timeout`
defaults to **15 minutes** (set `0` to disable reaping entirely); the two caps
default to `0` (unlimited).

```yaml
idle_timeout: 15m
max_connections: 100
max_concurrent_requests: 32
```

Equivalent flags: `-idle-timeout 15m`, `-max-connections 100`,
`-max-concurrent-requests 32`.

- `idle_timeout` reaps connections that stop sending commands, bounding file
  descriptors and zombie-diagnosis noise. Pooled Redis clients reconnect
  transparently; a long-idle interactive session is closed and must reconnect.
- `max_connections` refuses connections at the cap; refused sockets are closed
  immediately without being counted.
- `max_concurrent_requests` answers `EMB`/`EMB.MULTI` with `ERR busy ...` while
  at the cap, giving consumers backpressure instead of unbounded queueing.
  Control commands (`PING`, `AUTH`, `EMB.READY`, `EMB.STATS`, `EMB.MODELS`,
  `EMB.INFO`, `EMB.HELP`) always answer so ops can still probe a saturated
  server.

### Observability

`EMB.STATS` reports uptime, total requests, live `connections` and
`active_requests` (the real in-flight count), a per-model breakdown
(requests, avg latency, tokens, errors, pooling), live process resources
(`mem` = RSS MB, `cpu_user_usec`/`cpu_sys_usec`, `goroutines`), the cache
counters, and the effective
`idle_timeout_ms`/`max_connections`/`max_concurrent_requests` — a 10-second
check to classify a CPU/stuck-traffic incident as volume, saturation, or
churn (and to confirm no memory leak: RSS/goroutines flat between polls).

`MONITOR [seq] [limit]` exposes the last completed-request events (up to 8192,
oldest evicted) for per-request visibility: latency percentiles, error and
volume attribution per model. It is named after Redis's `MONITOR` but is a
bounded sequence-query rather than a long-lived stream — pass the last `seq`
you saw to fetch only newer events, which is how [`emb-top`](#monitoring-emb-top)
keeps its cost near zero. Request texts are never included.

`INFO [section...]` renders Redis-format sections; with no argument it returns
all of them:

- **# Server** — `redis_version`, `emb_version`, `uptime_secs`, `process_id`
- **# Cache** — hits, misses, hit rate, evictions, entries, byte usage
- **# Keyspace** — per-model `db0:model=…,keys=…,hits=…,misses=…,hit_rate=…`
- **# Stats** — `total_requests`, `total_tokens`, `total_errors`, `models_loaded`, `total_net_input_bytes`, `total_net_output_bytes`
- **# Memory** — `used_memory_rss_bytes` (process RSS incl. ONNX/CGo), `used_memory_heap_bytes`, `goroutines`, `total_system_memory_bytes`
- **# CPU** — `used_cpu_user_usec`, `used_cpu_sys_usec`, `gomaxprocs`
- **# Clients** — `active_requests`

`CONFIG GET [glob]` reads the live configuration registry (`CONFIG GET cache*`),
and `CONFIG SET` tunes it at runtime — including **live cache resizing**
(`CONFIG SET cache 1GB` evicts immediately) and swapping the `password` (affects
new connections only). Read-only parameters (listen address, TLS, models) are
reported by `GET` but rejected by `SET`. Both require authentication, matching
Redis semantics.

## Monitoring: emb-top

`emb-top` is a live terminal dashboard for a running `emb` node. It connects
over the Redis protocol and polls `EMB.MODELS` / `EMB.INFO <model>` /
`EMB.STATS` / `MONITOR` once per second — pipelined in a single round trip —
and renders:

- aggregate **req/s** and **p95 latency** stream charts,
- a model-activity **heatmap** (rows = models, columns = recent polls, color = req/s),
- per-model req/s, tok/s, err/s, **p50/p95/p99 latency** (from `MONITOR`
  events), and identity (dim, pooling, quantization, batching),
- cache hit ratio and hit/miss/eviction rates, RSS memory, CPU %, connections,
  active requests, goroutines, and a live event ticker,
- `AUTH` / TLS support, auto-reconnect with a connection-lost banner, and a
  headless `-once` mode for scripts and CI.

```
 emb-top v0.4.0 · localhost:6379 · uptime 3h22m · 4 models · poll 1s · 447 r/s · p95 86.0ms · ● connected
╭ req/s · models × recent polls ───────────────────────────────────────────────╮
│ minilm        ████▇▇▇▆▆▅▅▄▄▄▄▄▅▅▅▆▆▇▇████                                      │
│ bge-small-en… ▅▄▄▃▃▂▂▂▂▂▂▃▃▄▄▅▅▆▆▇▇█████                                       │
│ e5-base       ▂▂▃▃▃▄▅▅▆▆▇▇███████▇▆▆▅▅▄▄                                       │
│ gte-tiny      ▃▅█████████▅▅▃▃▃                                                 │
╰────────────────────────────────────────────────────────────────────────────────╯
╭ req/s ──────────────────────────╮╭ p95 latency ────────────────────╮
│       ╭──╮                     ││    ╭╮    ╭─╮                   │
│ ╭─────╯  ╰──╮                  ││   ╯ ╰────╯ ╰──╮                │
│ ╯            ╰──╮              ││ ╯              ╰               │
╰────────────────────────────────╯╰────────────────────────────────╯
minilm        ████▇▇▇▆▆▅▅▄▄▄▄▄▅▅▅▆▆▇▇████  280 r/s  11.8k t/s  p50 6.5ms  p95 68.0ms  err 0
   dim 384 · mean · int8 · batch 32/16384 workers 2
bge-small-en… ▅▄▄▃▃▂▂▂▂▂▂▃▃▄▄▅▅▆▆▇▇█████  151 r/s   5.7k t/s  p50 8.8ms  p95 77.0ms  err 18 ↑
   dim 384 · cls · fp32 · batch 32/16384 workers 2
cache 93.6% ██████████  cpu 50.3% ██████  mem 552MB ████████████
conns 7 · active 1 · goroutines 22 · truncated 0/0
event bge-small-en-… · 2 texts · 12.7ms ✓
q quit · p pause · r reset · j/k scroll · ? help
```

```bash
# watch a node
emb-top -addr localhost:6379

# headless: machine-readable lines (rates + latency percentiles from MONITOR)
emb-top -addr localhost:6379 -once -samples 10 -interval 1s
# t=... total_requests=6 req_rate=2.0 tok_rate=11.0 cpu_pct=14.5 lat_p50_us=1139 lat_p95_us=1801 …

# secured node
emb-top -addr localhost:6379 -password secret -tls
```

Keys: `q` quit · `p`/space pause · `r` reset window · `j`/`k` scroll models ·
`?` help. Flags: `-addr`, `-interval`, `-password`, `-tls`, `-window`,
`-once -samples N`. Prefer the `EMB_TOP_PASSWORD` environment variable over
`-password` (command-line arguments are visible in process listings); sending a
password to a non-loopback address without `-tls` prints a warning. Terminal:
UTF-8; a color-capable terminal is recommended.

It ships inside the Docker image (`/usr/local/bin/emb-top`) and the
`emb-server` gem (`bin/emb-top`). It requires no `onnxruntime` — it is a pure
RESP client and builds with `CGO_ENABLED=0`.

## Clients

The response is raw little-endian float32 bytes by default, so any Redis
client works. To avoid decoding raw floats, ask for `VALUES` (decimal
values); under `HELLO 3` the values come back as typed doubles:

**Ruby (raw RESP2):**

```ruby
require "redis_client"

redis = RedisClient.new(port: 6379)
raw = redis.call("EMB", "minilm", "hello world")
emb = raw.unpack("e*")

# decimal reply (no binary unpack needed):
envelope = redis.call("EMB", "minilm", "VALUES", "hello world")
# => ["dtype", "FLOAT", "shape", [1, 384], "values", ["-0.1974...", ...]]
```

**Python (RESP3, typed doubles):**

```python
import redis
r = redis.Redis(port=6379, protocol=3)  # sends HELLO 3
env = r.execute_command("EMB", "minilm", "VALUES", "hello world")
# env => {b"dtype": b"FLOAT", b"shape": [1, 384], b"values": [-0.1974...]}
```

Or use the [`emb`](gems/emb/README.md) gem — connection pooling, proxy, and
multi-model support with automatic float32 decoding:

```ruby
require "emb"

Emb[:minilm]["hello world"]
# => [0.0123, -0.0456, 0.0789, ...]
```

Images use the same proxy and float32 decode; pass the encoded file bytes
unchanged (they are sent as an `ASCII-8BIT` RESP bulk):

```ruby
Emb[:clip].image(File.binread("cat.jpg"))
# => [0.0123, -0.0456, ...]

# several images: one vector per requested slot, nil for a failed/truncated one
Emb[:clip].image(File.binread("cat.jpg"), File.binread("dog.png"))
# => [[0.01, ...], [0.07, ...]]
```

**Python:**

```python
import struct
raw = redis.execute_command("EMB", "minilm", "hello world")
emb = list(struct.unpack(f"<{len(raw)//4}f", raw))
```

**Go:**

```go
var vec []float32
binary.Read(bytes.NewReader(raw), binary.LittleEndian, &vec)
```

Ruby gems:

- [`emb`](https://rubygems.org/gems/emb) — client library ([README](gems/emb/README.md))
- [`emb-server`](https://rubygems.org/gems/emb-server) — precompiled server binary ([README](gems/emb-server/README.md))

## Development

```bash
just format          # Format all Go code (gofmt + goimports)
just lint            # Linters (golangci-lint + go vet)
just test            # Run tests
just deadcode        # Fail on unreachable production functions (see deadcode-allow.txt)
just cover           # Per-package statement coverage + total
just bench           # Run Go benchmarks
just bench-all       # redis-benchmark suite (see BENCHMARK.md)
just bench-ruby      # End-to-end Ruby client harness (lazy-mode mechanisms) against a live server
just bench-ruby-multi # Same harness across TWO partitioned emb instances (url-array scenarios)
just build           # Build the emb binary
just dev             # Build and run the server
just download-model  # Download a model from HuggingFace
just verify-harness  # Unit-test the shared verification harness (no server/model/ONNX)
just verify-embeddings # Compare served embeddings to a Python reference (needs a model)
just verify-emb-multi  # EMB.MULTI byte-equality vs sequential EMB (two models)
```

Nix provides a reproducible dev shell with Go, ONNX Runtime, golangci-lint,
just, and all CGo configuration:

```bash
nix develop
```

Docker:

```bash
# Run with a model mounted:
docker run -v ./models:/models elcuervo/emb \
  -config /models/config.yaml
```
