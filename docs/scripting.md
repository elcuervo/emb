# Custom scripts

Lua scripts wrap the model call with your own preprocessing and
postprocessing, replying through the same RESP grammar as the embed path.
Commands: `EMB.EVAL`, `EMB.EVSHA`, `EMB.SCRIPT LOAD|EXISTS|FLUSH` — see
[Commands](commands.md).

## The model, as a function

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

## The script surface

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

`emb.embed` and `emb.image.embed` are available only for models that configure
an embedding (`dim` + `pooling != none`) or image branch respectively; on any
other model the function is absent, exactly like `emb.image` on a text-only
model. `emb.image.embed` runs through the same image session pool and
content-addressed cache as `EMB.IMG`, so an image embedded by either path is a
cache hit for the other. Image sessions open lazily on first
`emb.image.embed`; `emb.image.info` and `emb.image.preprocess` need only the
preprocessing plan, so a script that never touches the image surface allocates
no image resources.

Replies convert through the standard grammar: Lua string → bulk, list → array,
string-keyed table → hash (flat field/value pairs), `{err = "..."}` → error
reply. `EMB.HELP` lists the full surface.

## Input specs

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
`emb.run` (see
[`examples/scripts/snippets/image_zeroshot.lua`](../examples/scripts/snippets/image_zeroshot.lua)):

```lua
local spec = emb.image.preprocess(KEYS[1])   -- KEYS[1] = raw image bytes
local out  = emb.run({ [spec.input] = { shape = spec.shape, bytes = spec.bytes, dtype = spec.dtype } })
```

Every `emb.run` / `emb.run_batch` output carries its `dtype` in both forms
(`{shape, data, dtype}` and the packed `{shape, bytes, dtype}`), so any output
can be fed straight back as an input spec without the server re-inferring the
dtype from element values.

## JSON values and math semantics

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

## Packed and selective outputs

`emb.run` and `emb.run_batch` take an options table as their final argument:

- `{bytes = true}` returns each output as `{shape = {...}, bytes = <string>, dtype = "f32"|"i64"}`
  — one Lua string of little-endian raw elements, the exact inverse of the
  `bytes` input form. Nothing is materialized element-by-element, which is what
  keeps large graph outputs cheap. Without the option each output is the
  equivalent array form `{shape = {...}, data = {...}, dtype = ...}`.
- `{outputs = {"logits"}}` materializes only the named outputs; an unknown name
  is an error listing what the graph produces.

Both forms are charged against the same per-evaluation tensor-element budget.

## Similarity and distance

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

## Production scripting

Scripts are a supported inference path, not just a demo surface. What that
requires in practice:

- **Determinism and caching.** Scripts are pure compute: `os`, `io`,
  `require`/loaders, coroutines and `math.random*` are stripped, so identical
  inputs always produce identical replies. Replies are cached per
  `(model, script SHA1, args, KEYS count, text)`, so each per-text output must
  depend only on those — not on sibling `KEYS` or its position in the request.
  An evaluation that repeats a text in `KEYS` bypasses the reply cache (one key
  cannot hold two element replies for the same text), so it always re-runs. For
  a **pairwise** operation (similarity, rerank, cross-encoder), put one operand
  in `KEYS` and the other in `ARGV` — the ARGV hash is part of the key, so
  distinct pairs stay distinct cache entries.
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

## Example 1 — vector embeddings (`model(input) → output`)

[`examples/scripts/snippets/siglip2.lua`](../examples/scripts/snippets/siglip2.lua)
re-implements the embed path on a fused CLIP export: the image branch is fed a
constant zero tensor (`fill`), the text embedding is L2-normalized, and the
reply is **one 3 KB bulk** byte-identical to a direct embed (768 float32s,
`unpack('e*')`) instead of 768 separate RESP bulks:

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

## Example 2 — custom classification (`model(fn(input)) → output`)

[`examples/scripts/snippets/sst2.lua`](../examples/scripts/snippets/sst2.lua)
turns a raw-logits text model into a labeled classifier with the building
blocks: encode with the model's own tokenizer, run once, softmax–argmax against
labels passed as `ARGV`:

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
([`examples/scripts/reference/`](../examples/scripts/reference/), exercised by
CI against a downloaded model) and **illustrative snippets**
([`examples/scripts/snippets/`](../examples/scripts/snippets/)). The GLiNER2
span extractor
([`reference/gliner2.lua`](../examples/scripts/reference/gliner2.lua)) is the
maintained reference: it reads its logits tensor in packed form and decodes with
`emb.math.gather` + `emb.math.sigmoid`. The snippets cover extractive QA
(`qa.lua` — pair encode + constrained span search over offsets), reranking
(`rerank.lua` — per-document batched sigmoid scores), sequence classification
(`sst2.lua`), and cross-modal zero-shot (`image_zeroshot.lua`). See
[`examples/scripts/README.md`](../examples/scripts/README.md) for the full
table.

For the whole loop rather than one construct, see
[`examples/kitchensink/`](../examples/kitchensink/): a runnable application that
embeds a corpus with an `emb` instance, stores the vectors in a Redis vector
set, and answers ranked queries — `emb` computing, Redis storing and searching,
two servers and one protocol between them.

## Writing your own script

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

Sandbox, resource, and observability behavior is described in
[Production scripting](#production-scripting).
