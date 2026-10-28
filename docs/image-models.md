# Image models

How to serve an image embedding model — including the DINOv3 family, whose
published ONNX exports need no re-export — and what the `image:` block means.
For the image commands themselves see [Commands](commands.md#image-embeddings-embimg);
for every config key see [Configuration](configuration.md).

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
| `select` | how rows are reduced: `none` (already pooled, rank 2), `cls` (first token), `mean` (masked mean), `tokens` (every row from `start`, L2-normalized per row). Unset uses the model's `pooling`, else the tensor rank (rank 2 → `none`, rank 3 → `cls`) |
| `start` | first token row of a `tokens` output. Unset uses the model's own `num_register_tokens` (`start = 1 + registers`), else 0 |
| `grid` | patch edge of a `tokens` output; the emitted row count is `grid²`. Unset uses `size / patch_size` from the model's `config.json` |

Load-time validation names the output: a missing tensor, a `select` that
contradicts the tensor's rank, a `tokens` output with no resolvable grid, a grid
that does not divide `size`, or a duplicate name fails the boot. Row counts are
re-verified against the actual tensor at request time, so a wrong `start` is
refused rather than silently shifting the grid.

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

## DINOv3

DINOv3's useful product is two tensors from one run of the same graph: a pooled
CLS vector (retrieval) and a grid of patch tokens (dense features, attention
maps, segmentation). emb serves both from one inference per image.

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

## Adding another family

A new image model should be a config + docs exercise, not a code change:

1. Reuse a preset if the preprocessing matches one, or declare mean/std/rescale/
   resample/crop explicitly.
2. Name each product in `image.outputs`; `select: tokens` for a patch grid.
3. Let `config.json` supply `patch_size` / `num_register_tokens` when the export
   ships one, and declare `grid`/`start` directly when it does not.
4. Boot the server and read the resolved-plan line: every field's source is
   printed, so an unexpected default is one line away from being found.
