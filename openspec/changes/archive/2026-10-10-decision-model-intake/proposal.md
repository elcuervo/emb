# Proposal

## Why

The decision-model category grew past laya: the same
`input_ids`/`attention_mask`/`marker_pos`/`marker_mask`/`qtype` contract is now exported by
several independent publishers, and the cheapest of them are int8. emb's intake path cannot
mount them. Verified against live repositories:

- `yehor-oleksiuk/laya-typed-decisions-onnx` publishes `model_int8.onnx` (506 MB vs the
  846 MB local `models/laya-typed/model.onnx`) whose graph declares exactly the five laya
  inputs and the two outputs the preset reads (`logits`, `act_logits`; it omits
  `last_hidden_state`, which `scripts/laya.lua` never requests). Under `quantize: auto` it is
  never selected: artifact resolution only knows `model_quantized.onnx` /
  `onnx/model_quantized.onnx` / `onnx/quantized/model.onnx`, and the alphabetical fallback
  picks `model_fp32.onnx` (`f` < `i`). The quantization *label* logic already treats a
  filename containing `int8` as int8, so resolution and labelling disagree.
- `onnx-community/laya-typed-decisions-ONNX` (and most current `onnx-community` exports)
  publish a small graph plus external weights: `onnx/model.onnx` 4.4 MB with
  `onnx/model.onnx_data` 1685 MB. The downloader selects files by the `.onnx` extension and
  transfers exactly one file, so the sidecar is never fetched and the graph cannot load.

Both are intake bugs, not model bugs: the preset and the wire contract already fit.

## What Changes

- Quantized-artifact resolution SHALL recognise int8-named members (`model_int8.onnx`,
  `onnx/model_int8.onnx`) in addition to the `model_quantized.onnx` family, and an
  int8-named artifact SHALL outrank a sibling fp32 artifact.
- The downloader SHALL fetch the external-data sidecars of the selected graph
  (`<graph>.onnx_data` and its siblings in the same repository folder), so split exports load.
- The laya path SHALL be exercised against a graph that omits `last_hidden_state`, and its
  spec SHALL state which outputs the preset requires, so a reduced export is a supported
  mount rather than an accident.
- The next decision-model candidate SHALL be chosen from recorded evidence: a spike that
  dumps the ONNX input/output signature, sidecar layout, and quantization ops of the
  strongest published candidates, before any second preset is written.
- Docs and the model-download reference SHALL record the int8 named variants and the
  sidecar rule.
- An existing mount SHALL keep resolving the artifact it resolved before: the preview site's
  sandbox comes through exactly the path this change touches (`laya-real` is a `model_repo` +
  `model_subfolder` mount with `quantize` unset, i.e. `auto`), so the website SHALL be
  validated on the running stack, and its artifact SHALL be pinned by test.

Not in scope: a second decision-model preset (no candidate contract is verified yet), fp16
artifacts, any conversion or quantization performed by emb itself, and any change to what the
website sandbox mounts (the decision plate keeps the miniature plus the published
`typed-decisions` checkpoint).

## Capabilities

### New Capabilities
None.

### Modified Capabilities
- `huggingface-model-download`: quantized member naming widens to int8-named artifacts, and
  the downloader fetches ONNX external-data sidecars alongside the selected graph.
- `int8-weight-quantization`: selection recognises int8-named artifacts and prefers them over
  a sibling fp32 file; the quality requirement gains a decision-model tolerance (winning option
  matches, probabilities within 0.05 on a fixed question set) alongside the existing embedding
  cosine tolerance.
- `laya`: mounting a reduced third-party export is explicit — the graph contract requires the
  five inputs and the outputs the preset requests, and `last_hidden_state` is optional.

## Impact

- `internal/hfhub/hfhub.go` — `FindQuantizedONNX`, `DownloadModel` file resolution and
  transfer.
- `internal/registry/registry.go` — `resolveQuantize` / preferred-path list, quantization
  label consistency, `selectOutputTensor` note for scripted graphs.
- `internal/config/config.go` — `quantize` documentation.
- `scripts/laya.lua` — unchanged; it already requests `logits` and `act_logits`.
- `docs/`, `README.md`, `testdata/laya/`, `test-laya.yaml` — int8 mount and sidecar notes.
- `website/repl/sandbox.yaml` — unchanged, and verified unaffected: `codenamev/laya-onnx`
  ships no quantized or int8 member and no `.onnx_data`, so `laya-real` keeps resolving
  `typed-decisions/model.onnx`.
- Website validation on the composed stack (`just website-dev`: site + bridge + emb) and the
  published-tree checks (`just website`, `just website-ink`, `just website-presets-check`).
- No new dependencies, no wire-contract change, no command change.
