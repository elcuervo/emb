# Image models

How to serve an image embedding model — including the DINOv3 family, whose
published ONNX exports need no re-export — and what the `image:` block means.
For the image commands themselves see [Commands](commands.md#image-embeddings-embimg);
for every config key see [Configuration](configuration.md).

Three sections after [Preprocessing](#preprocessing) read the same two calls at
increasing depth — one call end to end, what one inference yields, what would
break it. Each stands on its own: stop after the first and you know what the
command does; reach the third and you know its limits. The per-model catalogs,
dialects and recipes follow them.

## What a model config declares

An `image:` block turns on `EMB.IMG` / `EMB.IMGMULTI` and names the products the
model's image branch serves:

```yaml
models:
  clip:
    onnx: ./models/clip/model_quantized.onnx
    tokenizer: ./models/clip/tokenizer.json
    normalize: true
    image:
      input: pixel_values        # the graph's image input
      size: 224                  # square input edge
      preset: clip               # preprocessing + layout profile
      outputs:
        - name: embedding        # the reply key clients read
          tensor: image_embeds   # the graph output it comes from
```

`outputs` is an ordered list of `{name, tensor, select, start, grid}`:

| field | meaning |
|---|---|
| `name` | reply key. Unique within the model; there is no "default" output, so a bare `EMB.IMG` serves all of them |
| `tensor` | graph output tensor to read |
| `select` | how rows are reduced: `none` (already pooled, rank 2), `cls` (first token), `mean` (masked mean), `tokens` (every row from `start`, L2-normalized per row), or `map` (a rank-3 `[1,H,W]` / rank-4 `[1,1,H,W]` tensor, emitted flat and never normalized). Unset uses the model's `pooling`, else the tensor rank (rank 2 → `none`, rank 3 → `cls`) |
| `start` | first token row of a `tokens` output. Unset uses the model's own `num_register_tokens` (`start = 1 + registers`), else 0 |
| `grid` | patch edge of a `tokens` output; the emitted row count is `grid²`. Unset uses `size / patch_size` from the model's `config.json` |

Load-time validation names the output: a missing tensor, a `select` that
contradicts the tensor's rank, a `tokens` output with no resolvable grid, a grid
that does not divide `size`, or a duplicate name fails the boot. Row counts are
re-verified against the actual tensor at request time, so a wrong `start` is
refused rather than silently shifting the grid. What each selector leaves on the
wire, and what it costs, is [Layer one](#layer-one-a-call-end-to-end) steps 5–7.

## Preprocessing

Resolved in a fixed order, each field independently: **explicit key →
`preprocessor_config.json` → preset → ONNX graph → built-in default**. The boot
log prints the resolved plan and the source of every field, so a wrong family
default is visible rather than silent:

```
  dinov3: image input="pixel_values" (explicit) size=224 (preprocessor_config.json) crop=none (preset:dinov3) resample=bilinear (preset:dinov3) ...
```

Presets (`image.preset`):

| preset | mean | std | rescale | resample | crop | layout |
|---|---|---|---|---|---|---|
| `clip` | `0.48145466, 0.4578275, 0.40821073` | `0.26862954, 0.26130258, 0.27577711` | 1/255 | bicubic | center | — |
| `siglip` | `0.5, 0.5, 0.5` | `0.5, 0.5, 0.5` | 1/255 | bicubic | none | — |
| `imagenet` | `0.485, 0.456, 0.406` | `0.229, 0.224, 0.225` | 1/255 | bicubic | none | — |
| `dinov3` | `0.485, 0.456, 0.406` | `0.229, 0.224, 0.225` | 1/255 | bilinear | none | `grid = size/patch_size`, `start = 1 + num_register_tokens` |

A preset only fills what nothing more specific states: an explicit key always
wins, and a repo's own `preprocessor_config.json` outranks the preset because it
is repo-specific truth.

Preprocessing itself is unchanged from the rest of the image path: decode →
resize (square squash unless `crop: center`) → rescale → per-channel normalize,
producing `[1, 3, size, size]` float32 RGB.

## Layer one: a call, end to end

The site's plate makes one call on one picture:

```
EMB.IMG depth BLOB OUTPUTS depth <image bytes>
```

What happens between a file and the numbers, in the order it happens. Every
figure below was measured on the sandbox at `127.0.0.1:6379` with the six
`demos/samples/parts/` photographs, using this command:

```bash
cat website/demos/samples/parts/pier-corridor.jpg | redis-cli -x EMB.IMG depth BLOB OUTPUTS depth
redis-cli EMB.INFO depth       # depth 266x266, select map, image_sessions 1
```

The same grammar serves any model a configuration exposes: the sections below
also cover the DINOv3 export, whose two products the server still publishes for
clients that configure it.

The wire sizes are the RESP reply's own byte count; the JSON sizes are what
`POST /api/exec` on the bridge at `127.0.0.1:8081` returns for the same bytes.

**1. The browser bounds the picture.** Only the site does this; a raw RESP
client joins at step 3. The plate draws the sample to a canvas at no more than
768 px on the long edge and re-encodes JPEG at quality 0.85 (`MAX_EDGE`,
`toBlob`). Those bytes — not the file in the repository — are what the model
sees, so a sample's stored size is not its request size. JSON carries no bytes,
so the encoded image is base64'd and named by index:

```json
{ "args": ["EMB.IMG", "depth", "BLOB", "OUTPUTS", "depth", "<base64>"],
  "bin": [5], "proto": 2 }
```

`pier-corridor.jpg` is 76 594 bytes in the repository; the request carries the
base64 of the plate's own bounded re-encode, a different file, so the request
body is 4/3 of that encode's size.

**2. The bridge admits one shape and re-encodes it.** `website/repl/allowlist.go`
accepts exactly `EMB.IMG <model> BLOB [OUTPUTS <name>[,...]] <one image>`, the
model `depth` (the sandbox's one image model), and every requested name that
model's config declares. `website/repl/bridge.go` then decodes argument 5 from
base64 back to bytes (refusing above `max_image_bytes: 262144`), and sends them
to emb as a binary-safe RESP bulk. Base64 exists for the JSON hop only; emb never
sees it.

**3. The server decodes, preprocesses, and hashes.** `internal/server/image.go`
takes SHA-256 of the bytes first and consults `img:<model>:<sha256>:<output>` for
each requested output. A miss is decoded (JPEG, PNG, GIF, WebP), squashed to
`size × size` in uint8, rescaled 1/255, normalized per channel, and stacked with
the other misses of the command into one `[n, 3, size, size]` float32 batch —
`266` for `depth`.

**4. One run answers every output.** `ImageResources.Embed` calls the session
once per batch chunk and reads every requested output from that one result map:
`predicted_depth` for `depth`, `pooler_output` and `last_hidden_state` for a
DINOv3 configuration. Two products are not two inferences. Measured on one
picture, cold, on `EMB.IMG dinov3 BLOB OUTPUTS embedding` against
`... OUTPUTS embedding,patches` (same pixels, a trailing byte added so the cache
could not answer): 33 072 µs and 34 902 µs. The grid is 401 KB of reply, not a
second pass.

**5. `select`, `start` and `grid` decide what leaves the graph.**

| output | tensor | `select` | what leaves |
|---|---|---|---|
| `embedding` | `pooler_output` | `none` (rank 2) | 1 × 384 |
| `patches` | `last_hidden_state` | `tokens`, `start: 5` | 196 × 384 — rows 5…200 |
| `depth` | `predicted_depth` | `map` | 266 × 266, flat |

`cls` is the same idea for a graph that publishes only `last_hidden_state`: it
keeps row 0 and drops the rest, so a 201 × 384 tensor leaves as 1 × 384.
`none` means the graph already pooled its rows.

`last_hidden_state` is 201 rows of 384: row 0 is the CLS token, rows 1–4 the four
register tokens, rows 5–200 the 196 patches. `start = 1 + num_register_tokens` =
5 drops the five rows that are not patches; `grid = size / patch_size` = 14 fixes
196 = 14². The count is re-checked against the tensor at request time — the rows
left after `start` must equal `rows` — so a wrong `start` is refused rather than
returning a grid shifted by one patch.

**6. `BLOB` packs the numbers with no header.** Raw little-endian float32,
row-major, one RESP bulk per output, keyed by the output name — no dtype, no
shape, no length. The shape is the model's, and the page reads it once from
`EMB.INFO` (`rows`, `dim`, `grid`).

**7. The reply's arithmetic.** Measured, `EMB.IMG ... BLOB` over RESP:

| call | payload | RESP framing | RESP reply | JSON to the browser |
|---|---|---|---|---|
| `dinov3` `embedding,patches` | 1 536 + 301 056 = 302 592 | 52 | 302 644 | 403 700 |
| `depth` `depth` | 266 × 266 × 4 = 283 024 | 26 | 283 050 | 377 509 |

The framing is the RESP3 map header and the name and length prefixes — `%2`,
`$9 embedding`, `$1536`, `$7 patches`, `$301056` and their CRLFs — 52 bytes for
two products, 26 for one. The bridge carries each bulk as base64 plus
`vector: {dtype: "float32", count: bytes/4}`, a length heuristic rather than a
header (the 1 536-byte vector reports 384, the grid 75 264), so the browser pays
4/3 of the payload: 403 700 bytes of JSON for `dinov3`, 377 509 for `depth`.

**8. The cache identity is the content.** The key is
`img:<model>:<sha256(bytes)>:<output>`: one entry per output, so a two-output call
moves the counters by two and the two models never share an entry. Measured
across the samples in `demos/samples/parts/`, a first call moved `cache_misses`
35 → 37 and answered in 33 926–38 673 µs of server time; the same bytes again
moved `cache_hits` 372 → 374, answered in 342–447 µs, and returned a
byte-identical reply. `depth` measured 55 645 µs cold and 264 µs warm on a fresh
PNG. The identity is the bytes, not the picture: re-encoding the same photograph
at another quality is a different key and a fresh run.

## Layer two: what one inference yields

**One pass, one space.** `pooler_output` is 1 × 384; `last_hidden_state` rows
5…200 are 196 × 384. The pooled vector is the same width as a patch vector
because it is the same graph — row 0 before pooling. So one run yields a
picture-level point and a part-level grid in one 384-d space: rank whole pictures
by the vector, rank parts by the grid, without re-running anything.

**The patch grid is an index nobody had to build.** With `normalize: true` every
row is a unit vector, so a dot product is a cosine. Inside one picture that is a
patch-to-patch similarity field — a client's own index over the reply. Across
two pictures it is still a cosine, because both grids live in the same 384-d
space and the same 14 × 14 lattice, so a patch from one photograph is comparable
with a patch from another by position and by content. There is no index to build
and no second pass to build it: the 196 rows *are* the index, and they arrive
with the vector they belong to.

**A map is a different kind of output.** `select: map` does not reduce rows: it
emits the whole `[1,H,W]` or `[1,1,H,W]` tensor flat — 266 × 266 = 70 756
numbers — and `EMB.INFO` reports `rows: 266, dim: 266` instead of a grid. Those
numbers are never normalized and are not a unit vector: a dot product between two
maps means nothing, and the declared shape is the whole contract. The client owns
the transform.

**Why depth is its own graph.** A dense feature is not a distance. A principal
component of `last_hidden_state` is an axis of variation — it orders patches by
appearance, not by metres, and no rotation of the 384-d space turns it into a
depth map. Monocular depth needs a decoder trained for it, which is why the plate
runs one depth model and nothing else. That model is not free: its own session
(`EMB.INFO depth` → `image_sessions: 1`, `intra_op_threads: 4`), 27 MB of int8
weights on disk and ~100 MB resident once the CPU provider dequantizes them
(`website/repl/sandbox.yaml`). A DINOv3 configuration beside it would add its own
22 MB of int8 weights and ~90 MB resident for a second vocabulary the plate no
longer asks for.

## Layer three: what would break it

- **No metres.** The map is monocular and relative — inverse depth, one image,
  no camera intrinsics. Nothing in the reply is a distance, and nothing in the
  reply is comparable across pictures: the page normalizes by percentiles, so
  two photographs come back on two unrelated scales.
- **One face.** The bridge admits exactly one image per call, and the two calls
  are independent. Two views of the same scene would come back as two maps with
  no shared scale, so nothing here supports stereo, multi-view consistency or a
  change in viewpoint.
- **224 against 336.** 196 rows of 384 (294 KiB) against 441 rows of 384
  (661 KiB), for 3× the resident weights.
  The 336 export is also static batch 1, so it cannot batch.
- **The uint8-resize parity gap.** emb resizes in uint8 and then rescales; the
  published processors rescale first. Cosine against independent references is
  ≥ 0.999 at 224 and ≈ 0.997 at 336 on the 40 × 30 fixture, because an 8.4×
  upscale amplifies a one-LSB input difference. Same pipeline, different
  sensitivity — not a wiring error.
- **Normalisation is the model's, not the request's.** `normalize: true` (set for
  `dinov3`, unset for `depth`) L2-normalizes the pooled vector and every patch
  row; a map is never touched. Turn it off and a client's cosine — a dot
  product over the returned rows — stops being a cosine, because the rows are no
  longer unit vectors.
- **The space is per checkpoint.** Two grids are comparable because both came
  out of the same graph at the same patch size and the same 384 dimensions; ask
  a different DINOv3 size and the numbers are not comparable, and a pooled vector
  from `ViT-B/16` (768) cannot be compared with one from `ViT-S/16` (384) at all.
- **`BLOB` on the wire.** 302 644 bytes of RESP become 403 700 bytes of JSON for
  `dinov3` and 283 050 become 377 509 for `depth`: 4/3 for the base64, plus a
  decode pass over the payload in the browser. `VALUES` is ~4.5× the payload
  again, which is why it is not the default for a grid.

**Refusals, measured.** Each one names the shape that works:

| attempted | refused by | reply |
|---|---|---|
| `OUTPUTS depth` on `dinov3` | server | `-ERR unknown image output "depth"; configured outputs: embedding, patches` |
| `VALUES` anywhere in the image family | bridge | `EMB.IMG accepts only <model> BLOB [OUTPUTS <name>[,...]] with one image, for the sandbox's depth models` |
| an image model the sandbox does not serve | bridge | `EMB.IMG serves only the sandbox's own image models (depth), not "dinov3"` |
| a second image | bridge | `EMB.IMG takes exactly one image per call` |
| `EMB.IMGMULTI` with image bytes | bridge | `binary arguments are accepted only by the sandbox's image or audio preset` |
| a map whose tensor is not `size × size` | server | refused rather than shifted; `start` or `grid` on a map fails the boot |
| `VALUES` on a map | server | `-ERR image output "depth" is a map; request it with BLOB (70756 float32 values)` — decided before the cache or any run |

## DINOv3

DINOv3's useful product is two tensors from one run of the same graph: a pooled
CLS vector (retrieval) and a grid of patch tokens (dense features, attention
maps, segmentation). emb serves both from one inference per image —
[Layer two](#layer-two-what-one-inference-yields) says what that one run buys,
and [Layer one](#layer-one-a-call-end-to-end) what the two calls carry.

### Catalog

ViT family (patch 16, `num_register_tokens: 4`, `image_size: 224`):

| Model | `hidden_size` | layers | ONNX export |
|---|---|---|---|
| ViT-S/16 | 384 | 12 | `onnx-community/dinov3-vits16-pretrain-lvd1689m-ONNX` |
| ViT-S+/16 (gated MLP) | 384 | 12 | `onnx-community/dinov3-vits16plus-pretrain-lvd1689m-ONNX` |
| ViT-B/16 | 768 | 12 | `onnx-community/dinov3-vitb16-pretrain-lvd1689m-ONNX` |
| ViT-L/16 | 1024 | 24 | `onnx-community/dinov3-vitl16-pretrain-lvd1689m-ONNX` |
| ViT-H+/16 (gated MLP) | 1280 | 32 | `onnx-community/dinov3-vith16plus-pretrain-lvd1689m-ONNX` |
| ViT-L/16 SAT-493M | 1024 | 24 | `onnx-community/dinov3-vitl16-pretrain-sat493m-ONNX` |

ConvNeXt family (single pooled output, no patch grid):

| Model | `hidden_sizes` | `depths` | ONNX export |
|---|---|---|---|
| ConvNeXt-T | 96/192/384/768 | 3/3/9/3 | `onnx-community/dinov3-convnext-tiny-pretrain-lvd1689m-ONNX` |
| ConvNeXt-S | 96/192/384/768 | 3/3/27/3 | `onnx-community/dinov3-convnext-small-pretrain-lvd1689m-ONNX` |
| ConvNeXt-B | 128/256/512/1024 | 3/3/27/3 | `onnx-community/dinov3-convnext-base-pretrain-lvd1689m-ONNX` |
| ConvNeXt-L | 192/384/768/1536 | 3/3/27/3 | `onnx-community/dinov3-convnext-large-pretrain-lvd1689m-ONNX` |

Every `onnx-community` repo also publishes quantized members
(`onnx/model_quantized.onnx`, `onnx/model_q4.onnx`), each with its own `_data`
sidecar; `quantize: auto` picks the quantized graph and fetches the sidecar with
it. ViT-7B/16 has no published ONNX export; the dense-head exports
(`...-chmv2-dpt-head`) and the attention-score companion (`...-MHA-scores`,
whose `postprocess.onnx` is exactly a slice + per-row L2 normalize) are out of
scope — `select: tokens` already returns the rows they postprocess.

### The two dialects

Verified against the downloaded graphs (`onnx.load(load_external_data=False)`)
and their `config.json`/`preprocessor_config.json`:

```
onnx-community (transformers.js, opset 20, dynamic axes)
  input    pixel_values       [batch, 3, 224, 224]      symbolic batch
  outputs  pooler_output      [batch, 384]              pooled CLS vector
           last_hidden_state  [batch, 201, 384]         1 CLS + 4 registers + 196 patches
  weights  186 of 213 initializers external, location "model.onnx_data"
  metadata config.json: patch_size 16, num_register_tokens 4, image_size 224
           preprocessor_config.json: size 224x224, bilinear (resample 2),
           do_center_crop null, ImageNet mean/std, rescale 1/255

kornia (vision-rt, opset 18, exported static)
  input    input              [1, 3, 336, 336]          batch fixed at 1
  outputs  descriptor         [1, 384]
           tokens             [1, 446, 384]             1 CLS + 4 registers + 441
  weights  188 of 206 initializers external, location "dinov3-vits16-336.onnx.data"
  metadata none shipped — declare size 336 and the layout explicitly
```

Both are DINOv3 ViT-S/16, and they name their outputs differently: that is why
outputs are configured by name and layout comes from the model's own metadata.

### Recipes

Download either one (graph, sidecar, and any metadata the repo ships):

```bash
just download-dinov3-model                                        # onnx-community ViT-S/16
just download-dinov3-model kornia/dinov3 ./models/dinov3-336      # kornia ViT-S/16 @336
```

```yaml
# onnx-community dialect: dynamic axes, self-describing metadata, both products
dinov3-vits16:
  onnx: models/dinov3-vits16/model.onnx
  normalize: true
  image:
    preset: dinov3
    input: pixel_values
    size: 224
    outputs:
      - {name: embedding, tensor: pooler_output}
      - {name: patches, tensor: last_hidden_state, select: tokens}   # start/grid from config.json
```

```yaml
# kornia dialect: static batch 1, no metadata beside the graph
dinov3-vits16-336:
  onnx: models/dinov3-336/dinov3-vits16-336.onnx
  normalize: true
  image:
    preset: dinov3
    input: input
    size: 336
    outputs:
      - {name: embedding, tensor: descriptor}
      - {name: patches, tensor: tokens, select: tokens, start: 5, grid: 21}
```

What the two dialects mean operationally:

- **External weights.** The weights live in a `<graph>_data` sidecar. Sessions
  are built from the graph's bytes, and the directory the graph was read from is
  handed to ONNX Runtime, so the sidecar is found beside the graph regardless of
  the process working directory. The sidecar's name comes from the graph, never
  from a convention.
- **Static batch.** The kornia graph declares batch 1. The server reads that
  axis and runs one image per session run, so a multi-image request still serves
  every slot; the onnx-community graph's symbolic axis batches normally.
- **No tokenizer.** A DINOv3 export ships none. A model that declares an
  `image:` block loads without `tokenizer.json`; a text command on it fails with
  an error naming the model and the missing tokenizer.
- **Opsets.** 20 (onnx-community) and 18 (kornia) are both within the ONNX
  Runtime the server links.
- **No fp16 for TensorRT.** The kornia card warns that TensorRT fp16 yields
  all-NaN because attention logits exceed fp16 range; the fp32 graphs served here
  are unaffected and emb builds no TensorRT engines.

### Reply sizes

`BLOB` is the documented default for grids; `VALUES` is ~4.5× larger (decimal
floats):

| model @ size | rows | dim | per-output `BLOB` |
|---|---|---|---|
| ViT-S/16 @224 | 196 | 384 | 294 KiB |
| ViT-S/16 @336 | 441 | 384 | 661 KiB |
| ViT-B/16 @224 | 196 | 768 | 588 KiB |
| ViT-L/16 @336 | 441 | 1024 | 1.7 MiB |

[Layer one](#layer-one-a-call-end-to-end) carries the arithmetic of the served
call: what those bytes cost in RESP, in base64 and in JSON.

The site serves the 224 dialect for exactly this reason. Its plate asks for both
products in one call — `EMB.IMG dinov3 BLOB OUTPUTS embedding,patches` — and one
image's reply is 301 056 bytes of `float32` (196 × 384) beside the 1 536-byte
pooled vector, carried base64 by the sandbox bridge and decoded in the browser.
The 336 dialect would triple that for a grid the page draws at 320 px, and its
fp32-only export costs three times the resident weights. The bridge admits that
one command shape and nothing else in the image family: `VALUES`, `EMB.IMGMULTI`,
another model, and a second image are refused.

### Parity and its limits

The published DINOv3 image processors run rescale → resize → normalize, while
emb resizes in uint8 and then rescales. Same mean/std, same square squash, same
bilinear family; the difference is one uint8 quantization before interpolation.

The repository's parity tests compare embeddings by cosine against references
generated independently with Pillow + onnxruntime
(`internal/registry/testdata/generate_dinov3_reference.py`). The 224 dialect
reaches ≥ 0.999; the 336 dialect reaches ≈ 0.997 on the 40×30 test fixture,
because that upscale is 8.4× and this model amplifies a one-LSB input
difference to ≈ 0.998 cosine on its own. That is a fixture-and-model
sensitivity limit, not a wiring error: at 224 the same pipeline clears 0.999.

### Licensing

DINOv3 weights and conversions are under the **DINOv3 License**
(`license: other`, `license_name: dinov3-license`,
<https://ai.meta.com/resources/models-and-libraries/dinov3-license>), which
restricts military/weapons use. emb downloads weights only when the operator
asks and never vendors them in the repository or in test fixtures; the reference
fixtures are numbers generated from a locally downloaded model.

## Depth maps

A depth model is not an embedding model: its product is a two-dimensional map,
one value per pixel, and the values are not a unit vector. `select: map` serves
it as packed `float32` in row-major order and reports the shape in `EMB.INFO`:

```yaml
models:
  depth:
    onnx: /data/models/int8/depth/model_quantized.onnx
    quantize: auto
    intra_op_threads: 4
    image:
      preset: imagenet
      size: 266
      outputs:
        - { name: depth, tensor: predicted_depth, select: map }
```

```
EMB.IMG depth BLOB <image bytes>
# -> one map of rows*dim float32 (266 x 266 = 283 024 bytes)
EMB.INFO depth
# -> image_outputs.depth = { tensor: "predicted_depth", select: "map", rows: 266, dim: 266 }
```

[Layer two](#layer-two-what-one-inference-yields) says why a map is a different
kind of output, and why depth needs a graph of its own.

- The map is **never normalized**: it is not a vector, and the client owns the
  transform (the site normalizes by percentiles and treats it as relative
  depth).
- `start` and `grid` describe token rows; a map output that declares either
  fails the boot. `VALUES` on a map is refused at request time by the server —
  `-ERR image output "depth" is a map; request it with BLOB` — before the cache
  or any run, and the sandbox bridge admits `BLOB` only for the whole image
  family (see [Layer three](#layer-three-what-would-break-it)).
- The map's declared shape is the configured `size` (a fully-convolutional head
  preserves its input edge). The request checks the tensor's real shape and
  refuses to read a map that disagrees, rather than returning a silently
  shifted picture.
- `size: 266` is 19 × 14, the multiple-of-14 rule of the Depth Anything export,
  and it keeps one reply at 283 024 bytes packed — 368 KiB of base64, 377 509
  bytes of bridge JSON. The 518 dialect would be 1.43 MiB for detail a particle
  field cannot show.

The site serves **Depth Anything V2 Small**
(`onnx-community/depth-anything-v2-small`, int8, 27 MB on disk) as its depth
entry: Apache-2.0, 25M parameters, relative (inverse) depth. The preprocessing
matches the `imagenet` preset — mean `[0.485, 0.456, 0.406]`, std
`[0.229, 0.224, 0.225]`, rescale 1/255, bicubic resize — because the export's
own `preprocessor_config.json` states exactly those values. The export's
`keep_aspect_ratio` hint is not implemented by emb; the square squash is
harmless for a client that samples the map by normalized coordinates.

Do not derive depth from DINOv3's features: a principal component is an axis of
variation, not a distance. That is why this is a second model —
[Layer two](#layer-two-what-one-inference-yields) works it through.

## Adding another family

A new image model should be a config + docs exercise, not a code change:

1. Reuse a preset if the preprocessing matches one, or declare mean/std/rescale/
   resample/crop explicitly.
2. Name each product in `image.outputs`; `select: tokens` for a patch grid.
3. Let `config.json` supply `patch_size` / `num_register_tokens` when the export
   ships one, and declare `grid`/`start` directly when it does not.
4. Boot the server and read the resolved-plan line: every field's source is
   printed, so an unexpected default is one line away from being found.
