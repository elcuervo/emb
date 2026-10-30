# Proposal

## Why

emb's image branch assumes a model produces exactly one vector per image. DINOv3's useful product is two tensors from a single run of the same graph: a pooled CLS vector (retrieval) and a grid of patch tokens (dense features, attention maps, segmentation). Its published ONNX exports also disagree with three load-time assumptions emb makes — weights live in an external sidecar while sessions are built from an in-memory byte slice, one export dialect fixes the batch axis at 1 while the server always sends a full batch, and the model ships no tokenizer while model validation requires one. Today such a model either fails to load or can only ever expose one of its two products.

## What Changes

- `image.output` (one tensor name) becomes `image.outputs`: an ordered list of named outputs, each `{name, tensor, select, start, grid}`. There is no "default" output; the list is what the model produces.
- `select` is `none | cls | mean | tokens` and defaults to the model's `pooling`; `tokens` emits every row from `start` onward, normalized per row. `grid` (the patch edge) is required for `tokens`, resolved from the output config, a preset, or the model's `config.json`, so the emitted row count is always checkable against `(size/grid)²`.
- `EMB.IMG` and `EMB.IMGMULTI` accept an optional `OUTPUTS <name>[,<n>...]` keyword between the reply-format keyword and the first image. Absent, **every** configured output is served.
- **BREAKING** the image reply is uniformly keyed by output name: one image replies a map of `name → value`, several images an array of such maps. `BLOB` values are the raw float32 bytes, `VALUES` values are `{dtype, shape, values}` envelopes that also carry `grid: [h, w]` for token outputs. `EMB.IMGMULTI` replies one such map per model/image pair, keeping its `model` field. There is no single-output compact form and no positional form to keep.
- `image.preset` names a family profile (`clip`, `siglip`, `imagenet`, `dinov3`) that fills preprocessing (mean/std/rescale/resample/crop) and token-layout (`start`, `grid`) fields the config and `preprocessor_config.json` leave unset; `patch_size` and `num_register_tokens` are also read from a model's `config.json`, so an ONNX export that ships its own metadata needs no constants in the model config.
- Load-time validation is per output: rank against `select`, `start`/`grid` against the emitted row count, and every image output's last dimension against the model dimension when the model also serves text.
- Image reply-cache entries become per-output: `img:<model>:<sha>:<output>`; per-model stats and `EMB.CACHE.FLUSH <model>` parse the model name from the same rule.
- Load fixes the published exports require: sessions created from bytes resolve external initializers from the model's directory; the per-chunk batch is capped by a static batch axis the graph declares (and left unbounded when the graph's batch axis is dynamic); an image-only model (an `image:` block and no `tokenizer.json`) loads without a tokenizer, and its text commands fail with an error naming the missing tokenizer.
- `EMB.INFO` gains `image_outputs`; `EMB.HELP` documents `OUTPUTS` and reserves the word; `emb.image.embed` returns a name-keyed table (or one value when `{output = "<name>"}` is given) and `emb.image.info()` reports the configured outputs.
- A new `docs/image-models.md` documents the DINOv3 ONNX family — variants, both export dialects, exact I/O, preprocessing, licensing — with a config recipe per dialect, because "add a model" must be a documentation exercise, not a code one.

## Capabilities

### New Capabilities

None: every behavior below extends a capability that already owns it.

### Modified Capabilities

- `image-embeddings`: named image outputs and presets, the `OUTPUTS` keyword, the uniformly keyed image reply (replacing the single-bulk `EMB.IMG command` requirement), per-output load validation, per-output cache identity, grid-aware token extraction, introspection, and requirements covering the published DINOv3 export dialects.
- `onnx-bytes-session`: byte-built sessions resolve a model's external initializers from its directory, so a graph whose weights live in a sidecar file loads.
- `model-loading`: the tokenizer becomes optional for a model that only serves images, and `image.preset` joins the auto-detection order.

## Impact

- **Code**: `internal/config` (image schema + strict decoding), `internal/registry` (output resolution, preset table, external data, static batch cap, tokenizer-optional load), `internal/pipeline` (token-row extraction), `internal/server` (leading-keyword parser, reply writers, per-output cache, `EMB.INFO`/`EMB.HELP`), `internal/script` (`emb.image.embed`/`emb.image.info`).
- **Wire**: `OUTPUTS` keyword plus a uniformly keyed image reply. Every image client changes how it reads a reply; none need to change what it sends.
- **Docs**: new `docs/image-models.md` (the DINOv3 catalog and per-dialect recipes), `docs/commands.md`, `docs/configuration.md`, `README.md`, `config.yaml`'s image example.
- **Clients**: the Ruby gem's image call returns a name-keyed hash, with an `outputs:` option to narrow it.
- **Fixtures**: download targets for both DINOv3 dialects plus Python-generated references (CLS cosine, one patch row), mirroring the SigLIP2 vision fixture.
- **Unchanged**: preprocessing math in `internal/imageproc`, request limits, admission/cpu policy, scripted sessions, text embedding paths.
