# Tasks

## 1. Subfolder download support

- [x] 1.1 Add `model_subfolder` to `ModelConfig` (`internal/config`) and resolve it through configuration parsing; verify a config round-trip with and without the field
- [x] 1.2 Teach `hfhub.DownloadModel` to resolve the ONNX and supporting files under the subfolder when it is set, writing them to the model directory under conventional names; verify with a test over a subfolder file listing and a missing-ONNX error
- [x] 1.3 Verify a real subfolder download end to end against `codenamev/laya-onnx` (`multilingual`), confirming `multilingual/model.onnx` and `multilingual/tokenizer/tokenizer.json` land at the configured paths

## 2. The real sandbox entry

- [x] 2.1 Add the real model entry to `website/repl/sandbox.yaml` (`model_repo: codenamev/laya-onnx`, `model_subfolder: multilingual`, volume paths, `max_length: 1024`) and preload `laya.lua` under it with the checkpoint's own envelope from `rl_agent_config.json`/`onnx_config.json`; verify the server boots and answers `EMB.EVSHA <real> <laya.lua sha> 1 <state> <questions>`
- [x] 2.2 Confirm the stamped `laya.lua` digest is accepted for the real entry as well as the miniature (the same preset bytes, two models); verify `python3 website/tools/stamp-presets.py --check` and a call against each model
- [x] 2.3 Exercise the real tokenizer through the preset (`SpecialTokenIDs`, `EncodePlain`) and confirm the sequence construction matches the reference; verify `TestLayaParityCorpus` and a manual call against the real tokenizer

## 3. Memory and sizing

- [x] 3.1 Measure the real checkpoint's steady RSS with the sandbox model set and record it in `website/repl/fly.toml`
- [x] 3.2 Either widen the machine to fit the fp16 export or produce and host an int8 export; if quantized, validate its answers against the fp16 export and state the measured drift
- [x] 3.3 Re-run `just sandbox-deploy` and confirm `/api/ready` after the first-boot download within the check's grace period

## 4. Plate routing and copy

- [x] 4.1 Route the Inbox and Quickstart calls to the real entry while the loops stay on the miniature; verify each example's `EMB.EVSHA` command names the right model in the commands disclosure
- [x] 4.2 Label each example's model in the plate (loops = shipped miniature, typed questions = the real checkpoint) and scope the stand-in caveat to the loops; verify the prose distinguishes them
- [x] 4.3 Re-verify the plate's four examples in the browser after the routing change: loops animate, typed questions show the real decisions, no console errors

## 5. Docs and specs

- [x] 5.1 Update `website/docs/index.html` §10 and the Laya section of `README.md` to name the real checkpoint, the subfolder download, and the two-model sandbox
- [x] 5.2 Update the plate's gallery entry and any "miniature" wording that no longer covers the typed-question examples

## 6. Integration

- [x] 6.1 Rebuild the decision-validation table (ticket, demo's claim, reply's decision) against the real checkpoint and confirm the decisions are sensible per ticket; verify the phishing and payroll tickets are no longer marked as they were with the miniature
- [x] 6.2 Run `just lint`, `just verify-harness`, the focused Go suites, and the site checks inside `nix develop`; verify all pass
- [x] 6.3 `just sandbox-deploy` and verify the live plate answers the Inbox with the real checkpoint
