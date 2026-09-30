# Tasks

## 1. Bool tensors in named-tensor IO

- [x] 1.1 Add `TensorBool` to `internal/onnx/named.go` (extend `TensorType`, `namedTensorValue`, `namedTensorFromValue` using `ort.Tensor[bool]`) and verify a unit test round-trips a bool tensor through a named session. 
- [x] 1.2 Add `b1`/`bool` to the Lua dtype whitelist in `internal/script/host.go` (`namedTensorFromLua` input parse + output dtype reporting, 1-byte packing) and verify a script can feed a bool input and read a bool output via `emb.run`. 
- [x] 1.3 Verify `just verify-harness` (CGO-free tests) still passes and `go vet ./...` is clean after the dtype change. 

## 2. Tokenizer primitives

- [x] 2.1 Extend the tokenizer interface with `SpecialTokenIDs()` (mask/cls/sep/pad) and `EncodePlain(text, maxLength)` (no special tokens) in `internal/tokenizer/tokenizer.go`, implemented in `reference.go` over `daulet/tokenizers` (discovery order: tokenizer-config special names → `added_tokens` → probes). 
- [x] 2.2 Add a test proving `SpecialTokenIDs` resolves from `added_tokens` and `EncodePlain` equals encode-without-specials for the minilm (or vendored laya) tokenizer.json, and that existing `Encode` behavior is unchanged (`go test ./internal/tokenizer/ -count=1`). 

## 3. Laya preset and parity harness

- [x] 3.1 Vendor ruby-laya's `test/fixtures/tiny/*` (model.onnx, onnx_config.json, rl_agent_config.json, expected.json, tokenizer/) into `testdata/laya/` and commit the parity corpus. 
- [x] 3.2 Write `scripts/laya.lua`: question validation, `build_sequence` template with marker accounting, budget/truncation (max_len/head_max_len, 48-token option cap, trim-options), per-bucket temperature clamp (0.5–5.0, non-number → 1.0), softmax, confidence `1−H(p)/log k`, answer assembly with `act = softmax(act_logits)[0]`, returning gem-shaped payloads. 
- [x] 3.3 Add a RESP round-trip parity test (serveScriptModel pattern from `internal/server/script_runtime_parity_test.go`) that mounts the vendored tiny model and asserts every `expected.json` case — single-option choice, 12-option `choice:11+` temperature clamp (0.1006→0.5), noul confidence, score, long-state truncation, `action.act_probability`, `input_tokens` — byte-matching the fixture payloads. 
- [x] 3.4 Add `test-laya.yaml` (example config mounting the export bundle with `script_preload`) and verify boot + `EMB.EVSHA` smoke test against the tiny model inside `nix develop`. 
- [x] 3.5 Document the operator flow in `README.md` ("Laya decision models" section: download an export from `codenamev/laya-onnx`, mount it, preload the script, call `EMB.EVSHA`), including real-checkpoint golden regeneration steps. 
- [x] 3.6 Move the checkpoint envelope onto the model entry: declare the budgets and temperatures as the `scripts` entry's `config` (the `script-config` capability) and expose them to `laya.lua` as `emb.script.config`, so the wire call drops `ARGV[2]`. `ARGV[2]` remains a per-call override, and the vendored parity corpus still passes. 
- [x] 3.7 Add the task-preset path alongside the generic preset: `scripts/snake.lua` (see `laya-live-loop`) is the first preset that owns a loop, proving the generic `laya.lua` and a task preset coexist without any model-specific host code.

## 4. Validation and integration

- [x] 4.1 Run `just lint`, `just test`, and `openspec validate emb-laya`; fix any failures. 
- [ ] 4.2 (Optional, needs a downloaded checkpoint) Regenerate golden answers from a real ModernBERT/mmBERT export and record any daulet/tokenizers divergence as a follow-up note.

## 5. Spec repair (pre-existing defect)

- [x] 5.1 Repair `openspec/specs/script-tensor-io/spec.md`: the main spec carries leaked delta headers (`## MODIFIED Requirements` at line 1, requirements outside `## Requirements`) from an earlier archive and is structurally invalid — validate currently can't parse it and archive refuses its deltas. Restore the main-spec shape (`# script-tensor-io Specification` / `## Purpose` / `## Requirements` wrapping the three existing requirements and scenarios) without changing requirement text, then verify `openspec validate emb-laya` and `openspec archive` dry-run accept it. Same repair pattern as `harden-0-4-0-release`'s script-eval/lru-cache fix. 