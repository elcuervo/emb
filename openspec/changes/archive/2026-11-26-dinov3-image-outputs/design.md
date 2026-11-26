# Design

## Context

See `proposal.md` — Why. Three facts from the current tree and two real export dialects shape the approach.

Current shape: `ImageResources` carries one `OutputTensor`, one model-level `Pooling`, one `Normalize`, one `Dim` (`internal/registry/image.go`); `Embed` always builds a `[n,3,S,S]` tensor; `pool` returns exactly one row per image; both reply writers are hardcoded to `dim`-sized rows (`internal/server/image.go`, `server.go`). Sessions are always created from an in-memory byte slice (`ort.NewDynamicAdvancedSessionWithONNXData`, `internal/onnx/named.go`), while IO introspection loads from the path (`ort.GetInputOutputInfo`), so shapes are readable even when weights are external. Config decoding is `yaml.Unmarshal` (non-strict, `internal/config/config.go`), so a renamed key is silently ignored rather than rejected.

The two DINOv3 ONNX dialects, verified by reading the graphs (see "Reference models" for provenance):

```
onnx-community (transformers.js, opset 20, prefers dynamic axes)
  input   pixel_values   [batch, 3, height, width]                      symbolic batch
  outputs last_hidden_state [batch, (H/16)*(W/16) + 5, 384]
          pooler_output     [batch, 384]
  weights 186 of 213 initializers external, location "model.onnx_data"

kornia (vision-rt, opset 18, exported static)
  input   input          [1, 3, 336, 336]                               batch fixed at 1
  outputs descriptor     [1, 384]
          tokens         [1, 446, 384]                                 1 CLS + 4 regs + 441
  weights 188 of 206 initializers external, location "dinov3-vits16-336.onnx.data"
```

Both are DINOv3 ViT-S/16 with `patch_size 16` and `num_register_tokens 4`, and they name their outputs differently. That is the whole argument for configuring outputs by name and deriving layout constants from the model's own `config.json`.

## Goals / Non-Goals

**Goals:**
- One image request returns any named subset of a model's outputs from a **single** session run.
- A model's whole product is expressible in config, and a new family is a preset table row plus docs.
- One reply shape for images, so a client never branches on how many outputs a model has.
- Both published DINOv3 dialects load unmodified: external weights, static or dynamic batch, no tokenizer.

**Non-Goals:**
- Unifying text and image output handling. The text path's single-output contract is baked into the batcher, determinism proofs, and reply cache; multi-output text is a separate change with a much larger blast radius.
- Per-request image size, non-square inputs, stride/coarse grids, reply dtypes (f16/int8), per-output normalization.
- Attention-score products (`dinov3-...-ONNX-MHA-scores` ships a separate postprocess graph that slices and normalizes; a client that wants attention maps is a different feature).
- Changing preprocessing math in `internal/imageproc`; shipping model weights (the DINOv3 License governs them — see "Reference models").

## Decisions

### D1. Projection is an `OUTPUTS` keyword on the existing commands

`EMB.IMG <model> [BLOB|VALUES] [OUTPUTS <n>[,<n>...]] <bytes>...`, and the same position in `EMB.IMGMULTI` (after its leading `BLOB|VALUES`). Absent → every configured output.

Alternatives rejected:
- **A separate dense command (`EMB.PATCHES`)** — a client wanting the vector *and* the grid for one image pays two session runs; it also duplicates writers, limits, truncation and the URL guard.
- **No keyword at all (always all outputs)** — simple, but a vector-only consumer has no way to avoid shipping a 661 KiB grid, and the reply cache becomes useless for the model that needs it most. The keyword costs one reserved word and is the only grammar change.
- **Model-dependent positional output names** — a typo becomes "not an image" at decode time, and parsing would depend on the model.

`outputs` joins `blob`/`values` in the reserved model-name check the config already enforces. The value is one comma-joined token, so the keyword always has a known length and never needs a count.

### D2. The image reply is uniformly keyed by output name

```
one image     -> a map: {output_name: value, ...}
several images -> an array of those maps, request order, null for a failed/truncated slot
value         -> BLOB: raw little-endian float32 bytes
                 VALUES: {dtype, shape, values [, grid]}
```

```
EMB.IMG dinov3 <jpg>                  -> {"embedding": <1536B>, "patches": <661KB>}
EMB.IMG dinov3 OUTPUTS patches <jpg>  -> {"patches": <661KB>}
EMB.IMG dinov3 <a> <b>                -> [ {"embedding":…, "patches":…}, {…} ]
```

`writePairs` already emits `WriteMap` under RESP3 and a flat name/value array under RESP2, matching the `MONITOR`/`EMB.INFO` precedent.

Alternatives rejected:
- **Compact single-output form (a bare bulk)** — it was the pre-change behaviour, and keeping it would make the reply shape depend on how many outputs a model happens to configure. Since the API is not frozen, one shape is worth more than a smaller reply header: a client reads `reply["embedding"]` whether the model has one output or five.
- **Always an array of tensors in request order** — anonymous; a client has to remember order, and `redis-cli` output stops explaining itself.
- **Single image → array of one** — needless nesting that contradicts the house rule the text commands follow (single argument → item, several → array).

The "one image → item" rule is kept because it is the project's existing convention for multi-argument commands, not for compatibility.

### D3. `image.outputs` is an ordered list with explicit names and tensors

```yaml
image:
  preset: dinov3
  input: pixel_values
  size: 224
  outputs:
    - name: embedding
      tensor: pooler_output
    - name: patches
      tensor: last_hidden_state
      select: tokens
      start: 5
      grid: 16
```

List order only fixes the order of RESP2's flat name/value pairs; it carries no meaning. There is no default output and no `default_output` key — a bare request serves the whole map.

Rejected alternatives: a map keyed by tensor name (order lost in Go, and ugly export names leak into client APIs), a map keyed by reply name with a string shorthand (two spellings for one field), and `default: true` markers (a second way to say something the reply already says).

Strict config decoding (`yaml.Decoder` with `KnownFields(true)`) lands with this change so the `output` → `outputs` rename fails loudly instead of silently falling back to `output_tensor`. Deliberate side effect: typos in any shipped YAML become boot errors.

### D4. Extraction is one primitives table, not a family branch

`select` reuses the existing `pooling` vocabulary and adds `tokens`:

| select | rank | rows per image | implementation |
|---|---|---|---|
| `none` | 2 | 1 | existing pre-pooled extraction |
| `cls` | 3 | 1 | existing CLS extraction |
| `mean` | 3 | 1 | existing mean-pool |
| `tokens` | 3 | `grid²` | **new**: slice rows `[start:seq)`, L2-normalize per row |

`grid` is mandatory for `tokens`, so `seq − start == grid²` is verified against the tensor before any row is written. That makes a wrong `start` (for example forgetting DINOv3's 4 register tokens) unservable rather than silently shifted.

The extraction semantics are the ecosystem's own: the `-MHA-scores` companion export ships a `postprocess.onnx` whose entire graph is `Slice(offset) → Mul → ReduceSum → Sqrt → Div` over `last_hidden_state` — an offset slice plus per-row L2 normalization, which is exactly `select: tokens, start: N, normalize`. No new vocabulary is being invented.

The new extractor mirrors `ExtractCLS`: one contiguous `rows*dim*4` blob per image, SIMD-normalized per row, with a scalar reference for tests. Per-image results stay a single `[]byte`, so only the writer slices them into per-output frames.

`select` also subsumes the model-level `pooling` for the image branch, so no `image.pooling` key is introduced; a text+image model keeps its text pooling and overrides per output.

### D5. Presets and model metadata fill what the config leaves out

Two fixed orders, because the sources carry different fields:

- preprocessing (mean/std/rescale/resample/crop): **explicit key → `preprocessor_config.json` → preset → ONNX graph → built-in default**;
- layout (`start`, `grid`): **explicit key → `config.json` (`num_register_tokens`, `patch_size`) → preset**.

A size that no source determines fails the load, and the resolved plan is logged with each field's source so a built-in default is never silent. Reading the layout from `config.json` is what makes the onnx-community exports nearly config-free: they declare `patch_size: 16` and `num_register_tokens: 4`, which resolve to `grid: size / patch_size` (14 at 224, 21 at 336) and `start: 5` with nothing in the model config. `patch_size` is the patch's edge in pixels, not the grid edge — verified against both downloads (`last_hidden_state` carries 201 rows at 224 = 1 + 4 + 196).

- Preset `dinov3` (preprocessing + layout): mean `[0.485, 0.456, 0.406]`, std `[0.229, 0.224, 0.225]`, rescale `1/255`, resample `bilinear`, crop `none`, `start = 1 + num_register_tokens`, `grid = size / patch_size`.
- For `onnx-community/dinov3-*` repos the shipped `preprocessor_config.json` already says exactly that (`size 224×224`, `do_center_crop: null`, `resample: 2`, ImageNet mean/std, `rescale_factor: 1/255`), so the preset mostly matters for exports that ship no preprocessing config at all — the kornia export ships none, where today the server silently inherits SigLIP's 0.5/0.5 mean/std.
- `preprocessor_config.json` outranks the preset because it is repo-specific truth; the preset exists for repos with none.

Rejected alternative: a per-family branch in `resolveImagePlan` — it makes adding a model a code change, which is what this change exists to avoid.

Parity gap that remains: DINOv3's processor runs rescale → resize → normalize, while emb resizes in uint8 then rescales. Same mean/std, same square squash, same bilinear family; the difference is one uint8 quantization before interpolation. Reference tests therefore assert cosine parity, not bit equality — the stance the SigLIP2 fixture already takes.

Measured bound (2026-10-28, `golden_input.png`, 40×30): the 224 dialect reaches cosine ≥ 0.999 against a Pillow-preprocessed reference, but the 336 dialect lands at 0.9971. The cause is the fixture, not the wiring: two independent resamplers that differ by one uint8 LSB produce 0.9979 cosine through this graph on their own, because DINOv3 amplifies unit-level input differences. The kornia-parity test therefore asserts ≥ 0.995 and records why; reaching 0.999 there would require reproducing emb's resampler in the reference, which would make the resize axis circular.

### D6. Cache keys gain an output segment

`img:<model>:<sha256(bytes)>:<output>`; `modelOf` returns everything before the trailing 64-hex segment, so per-model stats and `EMB.CACHE.FLUSH <model>` work for every output without a second parsing rule. A multi-output request consults and populates each output's entry independently.

Rejected alternative: refusing to cache dense outputs. It needs a rule the byte-budgeted LRU already makes unnecessary, and it would prevent `OUTPUTS patches` from ever being cheap on a repeat.

Per-image BLOB sizes the operator is choosing between:

| model @ size | rows | dim | per-output blob |
|---|---|---|---|
| ViT-S/16 @224 | 196 | 384 | 294 KiB |
| ViT-S/16 @336 | 441 | 384 | 661 KiB |
| ViT-B/16 @224 | 196 | 768 | 588 KiB |
| ViT-L/16 @336 | 441 | 1024 | 1.7 MiB |

`VALUES` is ~4.5× larger (decimal floats), so BLOB stays the documented default for grids.

### D7. Chunk sizing: outputs count, static axes cap, dynamic axes don't

Chunk = `min(maxImageBatchImages, floor(budget / max(inputFloats, Σ outputFloats per image)), graphBatchCap)`, where `graphBatchCap` is a *positive* statically declared batch dimension on the configured image input (`GetInputInfo`), or unbounded when the axis is symbolic. `max(…)` rather than `input + Σoutputs` keeps today's behaviour where outputs are smaller than the input while never under-counting a dense model. Every requested output is read from the same run of each chunk.

This is what distinguishes the dialects: the kornia graph declares batch 1 and must be fed one image per run, while the onnx-community graph declares a symbolic batch and batches normally. The cap is therefore data, not a constant.

Rejected alternative: a fixed element budget per output class — one more constant per class and still unable to express large models.

### D8. External weights via a session config entry, not path-based sessions

Byte-built sessions get `session.model_external_initializers_file_folder_path` set to the graph's directory when the directory is known. This keeps `onnx-bytes-session`'s single-constructor property and avoids a second session lifecycle. Both dialects record the sidecar's **location inside the graph** (`model.onnx_data`, `dinov3-vits16-336.onnx.data`), so the code must never hardcode a sidecar name — it only has to give ORT the directory.

**Spike result (verified against both downloaded graphs, ORT 1.26):** the entry works for from-memory sessions, but its name ends in `_path` — `session.model_external_initializers_file_folder` is silently ignored (ORT resolves the sidecar against the process working directory and fails with `External data path does not exist`). With the `_path` key, both graphs open from bytes with the working directory elsewhere and run their declared shapes: onnx-community `last_hidden_state [1,201,384]` / `pooler_output [1,384]` for `pixel_values [-1,3,-1,-1]`, kornia `tokens [1,446,384]` / `descriptor [1,384]` for `input [1,3,336,336]`. The path-constructor fallback is therefore not needed.

### D9. Tokenizer optional iff the model is image-only

`ModelConfig.Validate` requires a tokenizer only when `image` is nil; the first text command on an image-only model fails through the existing `sharedTokenizer` error path, naming the model. `registry.downloadModel` also skips the tokenizer fetch when the repo has none, so a `model_repo`-declared image model works without a local workaround.

## Reference models

Provenance: file lists and `config.json`/`preprocessor_config.json` read from the Hugging Face API; graph inputs/outputs/initializer counts read from the downloaded ONNX graphs with `onnx.load(load_external_data=False)`; layout (`1 CLS + N registers + patches`) from the transformers DINOv3 docs.

### ViT family (all `/16`, `num_register_tokens: 4`, `image_size: 224`)

| Model | `hidden_size` | layers | ONNX export (ungated) |
|---|---|---|---|
| ViT-S/16 (`facebook/dinov3-vits16-pretrain-lvd1689m`) | 384 | 12 | `onnx-community/dinov3-vits16-pretrain-lvd1689m-ONNX` |
| ViT-S+/16 (`...-vits16plus-...`, gated MLP) | 384 | 12 | `onnx-community/dinov3-vits16plus-pretrain-lvd1689m-ONNX` |
| ViT-B/16 (`...-vitb16-...`) | 768 | 12 | `onnx-community/dinov3-vitb16-pretrain-lvd1689m-ONNX` |
| ViT-L/16 (`...-vitl16-...`) | 1024 | 24 | `onnx-community/dinov3-vitl16-pretrain-lvd1689m-ONNX` |
| ViT-H+/16 (`...-vith16plus-...`, gated MLP) | 1280 | 32 | `onnx-community/dinov3-vith16plus-pretrain-lvd1689m-ONNX` |
| ViT-L/16 SAT-493M (`...-vitl16-pretrain-sat493m`) | 1024 | 24 | `onnx-community/dinov3-vitl16-pretrain-sat493m-ONNX` |
| ViT-7B/16 (`...-vit7b16-...`) | — | — | none published by `onnx-community`; not targeted |

`facebook/dinov3-vit7b16-pretrain-lvd1689m` exists but its config is not readable without auth, so this change makes no claim about it: the 1.2 MB graph/86 MB sidecar are not the constraint, the reply size is (see D6).

### ConvNeXt family (`dinov3_convnext`, `image_size: 224`, no patch grid)

| Model | `hidden_sizes` | `depths` | ONNX export |
|---|---|---|---|
| ConvNeXt-T | 96/192/384/768 | 3/3/9/3 | `onnx-community/dinov3-convnext-tiny-pretrain-lvd1689m-ONNX` |
| ConvNeXt-S | 96/192/384/768 | 3/3/27/3 | `onnx-community/dinov3-convnext-small-pretrain-lvd1689m-ONNX` |
| ConvNeXt-B | 128/256/512/1024 | 3/3/27/3 | `onnx-community/dinov3-convnext-base-pretrain-lvd1689m-ONNX` |
| ConvNeXt-L | 192/384/768/1536 | 3/3/27/3 | `onnx-community/dinov3-convnext-large-pretrain-lvd1689m-ONNX` |

ConvNeXt exports are single-output (`pooler_output`) and need no `tokens` projection; they are the control case proving the schema does not assume a patch grid. A dense-head export (`facebook/dinov3-vitl16-chmv2-dpt-head`, `onnx-community/dinov3-vitl16-chmv2-dpt-head-ONNX`) is out of scope for the same reason.

### What each dialect means for a model config

```yaml
# onnx-community dialect: dynamic axes, self-describing metadata, both products
dinov3-vits16:
  onnx: models/dinov3-vits16/onnx/model.onnx
  normalize: true
  image:
    preset: dinov3
    input: pixel_values
    size: 224
    outputs:
      - {name: embedding, tensor: pooler_output}
      - {name: patches, tensor: last_hidden_state, select: tokens}   # start/grid from config.json

# kornia dialect: static batch 1, no config.json, no preprocessor_config.json
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

Other export notes that matter operationally:

- Every `onnx-community` repo also publishes quantized members (`onnx/model_quantized.onnx`, `onnx/model_q4.onnx`, each with its own `_data` sidecar). `hfhub.QuantizedWeightNames` already lists the `model_quantized` family, so `quantize: auto` picks it; the sidecar must be fetched with it (`hfhub.DownloadExternalData` already derives `<graph>_data`).
- Opset 20 (onnx-community) and 18 (kornia) are both within the repository's ONNX Runtime 1.26.
- kornia's card warns that TensorRT fp16 produces all-NaN for this model because attention logits exceed fp16 range (2.1e6). That is a TensorRT-engine concern, not an ONNX Runtime one: the fp32 graphs emb loads are unaffected, and emb does not build TensorRT engines. It is recorded here so nobody “optimizes” by exporting fp16.
- Licensing: DINOv3 weights and conversions are under the **DINOv3 License** (`license: other`, `license_name: dinov3-license`, `https://ai.meta.com/resources/models-and-libraries/dinov3-license`), which restricts military/weapons use. The change therefore downloads weights at the operator's request and never vendors them in the repository or in test fixtures; the reference fixture is numbers generated from a locally downloaded model.

## Risks / Trade-offs

- **[ORT ignores the external-initializers session entry for from-memory models]** → spike task first; fallback is the path-based constructor (D8), isolated to one step.
- **[Two dialects, new ones appearing]** → names and layout are config, presets are a table, and `docs/image-models.md` records the catalog; a new variant needs a YAML block, not a branch.
- **[A keyed reply over RESP2 misread as a flat array]** → documented with a worked example; the gem returns a hash; the shape is the same one `MONITOR`/`EMB.INFO` already use.
- **[`VALUES` on a grid explodes the reply]** → BLOB is the default and the docs carry the size table.
- **[Strict config decoding rejects previously tolerated YAML]** → shipped configs and tests are exercised in the same change; the error names the offending key.
- **[A wrong `start` silently shifts a grid]** → impossible: `grid` is mandatory for `tokens` and the row count is validated against the tensor.
- **[Downgrade mis-attributes cache stats]** → an older build parses `img:<model>:<sha>:<output>` as model `img`. Stats-only; migration plan says flush before downgrading.
- **[Licensing mistakes in a public demo]** → the DINOv3 License is recorded in the new doc and the proposal; the repo ships no weights.

## Migration Plan

No compatibility shims: the API is not frozen, and this change replaces `image.output` and the compact image reply outright.

1. Config schema, strict decoding, validation — shipped YAML examples and docs updated in the same group, so a stale example fails the suite.
2. Load fixes and output resolution (external-data spike first, then static batch cap, tokenizer-optional), verified against both downloaded dialects.
3. Extraction + reply writers + cache keys, with fake-session tests for the reply matrix and the SigLIP2 one-output model proving a one-output model now returns a one-key map.
4. Script bindings, gem, docs (including `docs/image-models.md`), fixtures + references, and the parity runs.
5. Rollback: revert the change; before downgrading, `EMB.CACHE.FLUSH` (or boot with `cache: 0`) so entries in the new key shape are not carried over.

## Open Questions

- Whether `preset` should also supply `normalize` (it applies to the text branch too, so it stays model-level for now). Answering later is a purely additive key.
- Whether attention-score products (`-MHA-scores`, `postprocess.onnx`) should become a configuration surface or stay a client-side slice. The generic `select: tokens` already returns the rows they postprocess.
- Whether text `EMB` should ever accept `OUTPUTS` (it would require the text path to grow multi-output). No model in this change needs it.
