# Tasks

## 1. Verify the artifact before building on it

- [x] 1.1 Download `yehor-oleksiuk/laya-typed-decisions-onnx` `model_int8.onnx` + `tokenizer.json` into a scratch directory and load it under `nix develop` by mounting it as a `scripts`/`script_preload` model in a scratch config; verify the server boots, reports the model ready, and `EMB.EVSHA` answers a question. Record the observed result (load, opset, any ORT warning) in `design.md` under Risks. If it does not load, stop and report rather than continuing.
- [x] 1.2 Record the resolved-artifact probe for the candidates in Decision 6 (`Vela-Decision-170M`, `onnx-community/laya-typed-decisions-ONNX`, `ModernJEV-Decide-Preview`): input names, output names, opset, quantization domains, sidecar layout, and licence. Verify every row is filled from an actual graph or repository file listing, not from a model card. Artifact: a recorded table in `design.md`, and a one-line verdict per candidate on whether a preset is feasible.

## 2. Int8 artifact naming

- [x] 2.1 Add `model_int8.onnx` and `onnx/model_int8.onnx` to `FindQuantizedONNX` (`internal/hfhub/hfhub.go`) after the `model_quantized.onnx` family; verify with table-driven cases over `[]FileInfo`, including a repo holding both `model_fp32.onnx` and `model_int8.onnx` (expects the int8 member) and a repo holding only fp32 (expects `nil`).
- [x] 2.2 Add the same names to the preferred-path list in `resolveQuantize` (`internal/registry/registry.go`); verify a unit test that a directory holding `model_fp32.onnx` + `model_int8.onnx` under `quantize: auto` resolves to `model_int8.onnx`, `quantize: on` resolves to it, and `quantize: off` keeps fp32.
- [x] 2.3 Log the resolved weight file at model load (base name + resolved path) so an int8 miss is diagnosable; verify with a test asserting the log line on the int8 path and an integration check that `EMB.INFO` reports `quantization: int8` and the on-disk size for the int8 mount.
- [x] 2.4 Document the accepted quantized names and the `quantize` setting in `docs/configuration.md` and the `quantize` field comment in `internal/config/config.go`; verify the documented config snippet mounts the local `models/laya-typed` directory as written.

## 3. External-data sidecars

- [x] 3.1 Select sidecars from the repository file list in `DownloadModel` (`internal/hfhub/hfhub.go`): for the selected graph, the members `<graph>_data` in the same folder (which also covers an fp16 graph, whose sidecar follows the same rule); verify with a unit test over a fake file list that a split export selects exactly the matching sidecar and a self-contained graph selects none.
- [x] 3.2 Transfer each sidecar with its own existence check, independent of the graph's early return, publishing under its base name beside the graph; verify with an `httptest` server test: (a) a fresh download fetches graph + sidecar, (b) a directory that already holds the graph but not the sidecar still fetches the sidecar, (c) a failing sidecar transfer fails the download with an error naming it and publishes no partial file (the freshly downloaded graph is removed again, so the next boot retries instead of loading a graph with no weights).
- [x] 3.3 Document the sidecar rule and the split-export layout in `docs/configuration.md` (and the model-download section of `README.md`), including how to recover a cache written before this change; verify the documented layout is what the test in 3.2 publishes.

## 4. Reduced-output graph is a supported mount

- [x] 4.1 Mount the downloaded int8 export (`models/laya-int8`, `quantize: auto` resolving `model_int8.onnx`) beside the site's checkpoint in the parity harness, with its envelope carried by the preloaded preset; verify the server answers every question type over `EMB.EVSHA` with the graph that omits `last_hidden_state`.
- [x] 4.2 Add a `just verify-laya-int8` target that downloads the same checkpoint's int8 and fp32 artifacts, answers one fixed question set with both, and asserts identical winning options and probability deltas within 0.05; verify the target passes under `nix develop` and fails when the tolerance is tightened below the measured spread (i.e. the assert is real). Record the measured worst-case delta next to the tolerance (`0.0158` measured against `0.05`; `EMB_LAYA_INT8_TOL=0.01` fails).
- [x] 4.3 Update the laya section of `README.md` and `docs/configuration.md`: the int8 mount, the required outputs (`logits`, `act_logits`), that `last_hidden_state` is optional, and the non-deterministic scripted-output note from Decision 4; verify the documented `EMB.EVSHA` command runs as written.

## 5. Website validation

- [x] 5.1 Pin the site's artifact by test: a unit test over a `codenamev/laya-onnx`-shaped file list asserts `quantize: auto` resolves `typed-decisions/model.onnx` (no quantized member substituted, no sidecar expected). Verify the test fails if `model_int8.onnx` is added to that fixture's subfolder, so the pin is real.
- [x] 5.2 Run the composed stack — `just website-dev` (on ports 8090/8091/6390 so an existing stack keeps its own) — and verify the plate: all four examples answered through the bridge's `POST /api/exec` (Quickstart and Inbox from `laya-real`, Snake and Pac-Man 8 frames each from the miniature), the served page injected this sandbox's origin and carried the bridge's digests, and the Quickstart tab rendered the same numbers in the browser. `laya-real` resolved `models/laya-typed/model.onnx` (846075712 bytes) and `laya` the miniature, so no artifact moved.
- [x] 5.3 Verify the site's own checks stay green and the change stayed server-side: `just website-presets-check` → ok (20 markers, all 12 digests current), `git status --short website/` → empty, so no sandbox redeploy is needed. `just website-ink` passes on `demos/batch.html` and `docs` but **fails on `demos/laya.html`** at 2 of 24 widths (834, 320): pre-existing, no `website/` file changed here, recorded in `design.md` as a separate concern rather than fixed in this change.

## 6. Integration gates

- [x] 6.1 Run `just lint`, `just test`, `just deadcode`, and `just cover`; verify all pass and coverage stays at or above `coverage-floors.txt`.
- [x] 6.2 Boot one config that mounts the fp32 laya directory and the int8 export side by side, and verify both answer `EMB.EVSHA` in the same server process with `EMB.INFO` reporting each one's quantization and size.

## Workflow follow-up

- Archive the change after review, per the project's `openspec-archive-change` workflow.
- If task 1.2 finds a candidate whose contract matches laya's five inputs, open a follow-up change for its preset rather than widening this one.
- If a later change moves the sandbox onto an int8 decision export, that change owns the plate copy, the preset-digest restamp, and the hand `just sandbox-deploy` — none of it belongs to this change.
