# Tasks

## 1. Load the real exports (settle external data and the two dialects first)

- [x] 1.1 Spike the external-data question against both downloaded graphs (`onnx-community/dinov3-vits16-pretrain-lvd1689m-ONNX` records `model.onnx_data`; `kornia/dinov3` records `dinov3-vits16-336.onnx.data`, 188 external initializers). Load each from bytes in a throwaway program inside `nix develop` and prove whether `session.model_external_initializers_file_folder` resolves the sidecar. Verify: both sessions open and run with their declared input shapes; a failure is recorded in `design.md` D8 and the path-constructor fallback is taken.
- [x] 1.2 Thread the model's directory into the named-session constructor in `internal/onnx/named.go` and set the external-initializers folder there (or switch to the path constructor per 1.1), keeping one session-construction path and never hardcoding a sidecar name. Verify: `go test ./internal/onnx/` plus a test that opens a graph with a sidecar from bytes and runs it relative to a working directory that is not the model's.
- [x] 1.3 Cap the image chunk by a static batch axis: read `onnx.GetInputInfo` for the configured image input at load, keep a positive batch dimension as a cap and treat a symbolic axis as unbounded, then use `min(…, cap)` when chunking in `internal/server/image.go`. Verify: registry test for both a static (1) and a symbolic batch axis, plus a server test that a 3-image request against a batch-1 graph issues three runs and returns three slots.
- [x] 1.4 Make the tokenizer optional for image-only models: `ModelConfig.Validate` requires it only when `image` is nil, and `registry.downloadModel` skips the tokenizer fetch when the repo has none. Verify: config test that an image-only model without `tokenizer.json` loads; a server test that a text command on it fails naming the missing tokenizer.

## 2. Config schema, presets, and strict decoding

- [x] 2.1 Replace `ImageConfig.Output` with `Outputs []ImageOutputConfig{Name, Tensor, Select, Start, Grid}` and add `Preset`; validate unknown `select`, `tokens` without `grid` or on a rank-2 tensor, `grid` not dividing `size`, negative `start`, and duplicate names. Verify: table-driven `internal/config` tests for each rejection message.
- [x] 2.2 Switch config decoding to a strict `yaml.Decoder` (`KnownFields(true)`) and update every shipped YAML (`config.yaml` example, `test-*.yaml`, `bench-*.yaml`, `config/emb.yaml`) to the new `image.outputs` shape. Verify: `go test ./internal/config/` and a boot of `test-two-models.yaml` inside `nix develop`; a stale `image.output` key must fail with an error naming it.
- [x] 2.3 Add the preset table (`clip`, `siglip`, `imagenet`, `dinov3`) covering mean/std/rescale/resample/crop plus the DINOv3 layout defaults, resolved in the fixed order explicit → `preprocessor_config.json` → preset → graph. Verify: one unit test per preset and one asserting an explicit key overrides a preset field.
- [x] 2.4 Read `patch_size` and `num_register_tokens` from a model's `config.json` when the output config omits `grid`/`start` (deriving `start = 1 + registers`), leaving explicit values authoritative. Verify: a test against a `config.json` fixture with `patch_size: 16` and `num_register_tokens: 4` resolving `start=5` and `grid=16`, plus a test that an explicit `start` wins.
- [x] 2.5 Log the resolved plan with the source each field came from (preset name, `preprocessor_config.json`, `config.json`, or explicit), so a wrong family default is visible at boot. Verify: `just build` and a boot log showing the resolved image plan for a DINOv3-configured model.

## 3. Registry: outputs, validation, introspection

- [x] 3.1 Replace `ImageResources.OutputTensor/Pooling/Dim` with `Outputs []ImageOutput{Name, Tensor, Select, Start, Rows, Dim}`, resolving rank/dim from `onnx.GetOutputInfo` and failing load naming the output when a tensor is missing or a rank contradicts `select`. Verify: `internal/registry` tests for a missing tensor, a rank mismatch, and a duplicate name.
- [x] 3.2 Apply the dual-encoder pairing rule to every image output's last dimension, and keep `HasImageSurface`. Verify: registry test where one of two outputs disagrees with the model dim and load fails naming that output.
- [x] 3.3 Report the output list (name, tensor, select, rows/grid, dim) from the model entry for `EMB.INFO` and `emb.image.info()`, with no notion of a default output. Verify: `EMB.INFO` on a two-output fake model lists both with their shapes.

## 4. Token-row extraction

- [x] 4.1 Add `ExtractTokenRows(hidden, batch, dim, seq, start, normalize)` to `internal/pipeline/pooling.go`: one contiguous `rows*dim*4` blob per image, rows `[start:seq)`, L2-normalized per row when `normalize`, bit-identical to a scalar reference (mirroring the existing SIMD/scalar split). Verify: unit test against a scalar reference over a non-trivial (batch, seq, dim), including `start: 0` and both normalization settings.
- [x] 4.2 Change `ImageResources.Embed` to take the requested outputs, run once, and return one blob per (image, output), validating the batch dimension and `seq − start == grid²` before reading any row. Verify: fake-session tests where `seq` disagrees with `start`/`grid` and where the output batch disagrees, each failing with both counts named.

## 5. Server: parser, keyed reply, per-output cache

- [x] 5.1 Parse the leading keywords of `EMB.IMG`/`EMB.IMGMULTI` in one place (format, then `OUTPUTS <comma list>`), reject unknown output names listing the configured ones, dedupe repeats, default to every configured output, and skip the keyword arguments in the URL guard. Verify: server tests for each position, the unknown-name error, the all-outputs default, and a URL in the first image slot after `OUTPUTS`.
- [x] 5.2 Write the keyed reply: one image → a map of name to value, several → an array of maps, `null` for failed/truncated slots, `BLOB` values as raw float32 bytes, `VALUES` values as envelopes, `grid` on token envelopes, and one map per `EMB.IMGMULTI` pair (with `model` beside the outputs under `VALUES`). Verify: protocol-level tests asserting the exact RESP2 flat-array and RESP3 map shapes for one map with one key, one map with two keys, an array of two maps, and an `EMB.IMGMULTI` pair array.
- [x] 5.3 Cache per output: key `img:<model>:<sha>:<output>`, each requested output consulted and populated independently, and `modelOf` parsing the model before the trailing hex segment so stats and `EMB.CACHE.FLUSH <model>` keep working. Verify: cache tests for the new key shape and per-model attribution, plus a server test that a second identical two-output request performs no inference.
- [x] 5.4 Keep request accounting intact: `EMB.STATS` counts one request per command and one truncation per dropped image; `MONITOR` records one event. Verify: existing stats/monitor tests pass, plus a two-output request counted once.

## 6. Script bindings

- [x] 6.1 `emb.image.embed(bytes)` returns a table keyed by output name, `emb.image.embed(bytes, {output = "<name>"})` returns that single value, an unknown name raises an error listing the configured outputs, and `emb.image.info()` reports the output list. Verify: `internal/script` tests for the keyed table, the narrowed form, the token output's shape, and the unknown-name error.

## 7. Docs, client, fixtures

- [x] 7.1 Write `docs/image-models.md`: the DINOv3 catalog (ViT-S/S+/B/L/H+ and ConvNeXt T/S/B/L with hidden sizes, patch size, register count, and both ONNX repos), the two dialects with their verified I/O, a config recipe per dialect, the ImageNet preprocessing constants, the external-sidecar note, and the DINOv3 License. Verify: every repo and shape in the doc matches the downloaded graph/`config.json` it cites, and a config copied from the doc boots.
- [x] 7.2 Document the grammar and keyed reply in `docs/commands.md` (positions, reserved words, worked RESP2/RESP3 examples, the `VALUES` size caveat), the `image:` schema and preset table in `docs/configuration.md`, the `config.yaml` image example, and the README's image section. Verify: `grep -rn "image.output\|image: *$"` finds no stale shape, and the examples match the tests.
- [x] 7.3 Ruby gem: the image call returns a name-keyed hash and accepts `outputs:` to narrow it. Verify: `cd gems/emb && bundle exec rake` with a server on 16379 running a two-output fake config, and `bundle exec rubocop`.
- [x] 7.4 Add `just download-dinov3-model [repo] [dir]` covering both dialects: the graph, its `_data` sidecar as recorded by the repo listing, and `config.json`/`preprocessor_config.json` when present. Add `internal/registry/testdata/generate_dinov3_reference.py` writing the pooled row and one patch row for `internal/imageproc/testdata/golden_input.png`. Verify: both targets download a servable model, and the script writes reference files that a `numpy`-only reader can consume.

## 8. End-to-end parity

- [x] 8.1 Gated registry test over the onnx-community export: two outputs, `pooler_output` vector, `last_hidden_state` grid of 14x14 at 224, and CLS plus one patch row within cosine ≥ 0.999 of the generated reference. Verify: passes inside `nix develop` with the model present, and skips cleanly without it.
- [x] 8.2 Gated registry test over the kornia export: two outputs, `descriptor` vector, `tokens` grid of 21x21 at 336 with `start: 5`, one image per run despite the batch-1 graph, and both vectors within cosine ≥ 0.999 of the reference. Verify: same gating and tolerance.
- [x] 8.3 End-to-end server test against a real export: `EMB.IMG <model> <png>` returns a one-key map with a `dim*4`-byte value, `OUTPUTS embedding,patches` returns a two-key map whose `patches` value is `441*384*4` bytes, `VALUES patches` carries `shape [441, 384]` and `grid [21, 21]`, and a 3-image request serves all three. Verify: the test asserts sizes and shapes from an actual `resp` reply.
- [x] 8.4 Regression sweep: `just lint`, `just test`, `just deadcode`, `just verify-harness` inside `nix develop`. Verify: all green; no unreachable production function added; the SigLIP2 vision fixture keeps passing with the new keyed reply.

## Workflow follow-up

- Run `openspec validate dinov3-image-outputs` and archive the change with `/openspec-archive-change` once the implementation is committed as a bundle with its artifacts.
- Recheck `website/llms.txt` and the site's command reference for the `EMB.IMG` grammar and the keyed reply if it quotes the command table.
