# Proposal

## Why

The [ruby-laya](https://github.com/codenamev/ruby-laya) gem serves Laya — a multilingual, non-autoregressive System 1 decision engine — from ONNX exports of the published checkpoints (`codenamev/laya-onnx`: `english` ModernBERT-large, `multilingual` mmBERT-base, `typed-decisions` ModernBERT-large). `emb` sits on the same dependency shell (ONNX Runtime via CGo, `daulet/tokenizers`) and already serves non-embedding ONNX models through a Lua script host (`EMB.EVAL`/`EMB.EVSHA`, GLiNER precedent). Serving the same checkpoints gives `emb` a Redis-native decision path: typed answers (`choice`/`score`/`noul`) over any state in one forward pass, with the calibration the checkpoints ship. A council review of the two codebases confirmed the path is a small shared enabling slice (bool tensors + tokenizer primitives) plus one preloaded script preset pinned by a parity harness, not a new inference engine.

## What Changes

- **Go — bool tensors in named-tensor IO.** `NamedTensor` and the `emb.run` Lua surface gain a `bool` dtype (`marker_mask` in the Laya graph is an ONNX bool tensor; the underlying `yalue/onnxruntime_go` already supports it). This also unblocks any future bool-input model, not just Laya.
- **Go — tokenizer primitives for sequence construction.** The tokenizer exposes mask/cls/sep/pad token IDs (parsed from `tokenizer.json` `added_tokens`, per the Ruby gem's special-token discovery) and an encode-without-special-tokens variant, so scripts can reproduce Laya's `[CLS] … [SEP] [MASK] option … [SEP] state [SEP]` template exactly.
- **New — preloaded `laya.lua` preset.** A Lua preset that renders a request's `state` + typed questions into the graph's five inputs (`input_ids`, `attention_mask`, `marker_pos`, `marker_mask`, `qtype`), requests `logits`/`act_logits`, and applies the checkpoint's per-bucket temperature calibration, softmax, confidence (`1 − H(p)/log k`) and answer assembly — returning the same payload shapes as the Ruby gem.
- **New — model mount shape + example config.** A Laya checkpoint mounts like the GLiNER testbed (`onnx:` + `tokenizer:` + `script_preload`); an example `test-laya.yaml` documents the env vars for the export bundle's files (`rl_agent_config.json` temperatures, `onnx_config.json` budgets).
- **New — parity harness.** Ruby-laya's `test/fixtures/tiny/*` (32-hidden tiny model, `rl_agent_config.json` with out-of-range temperatures, `expected.json` corpus) is vendored and pinned through the live RESP surface, following emb's existing golden-corpus pattern. Optionally regenerated against a downloaded real checkpoint.
- **No new command surface.** Predictions are served over the existing `EMB.EVAL`/`EMB.EVSHA` commands: `KEYS[1]` = serialized state, `ARGV[1]` = questions JSON. The script reply cache already keys on args, so distinct question sets are distinct cache entries today.

Out of scope (future changes): a typed `EMB.PREDICT` command with Go-level validation (tipping condition: ≥2 independent clients needing a stable native contract), embedding-only serving of Laya (`last_hidden_state` mean-pooling), the router/language-detection heuristics, and fine-tuning.

## Capabilities

### New Capabilities
- `laya`: serve Laya decision checkpoints over the scripted-model path — typed answers with calibrated probabilities, parity-pinned against the Ruby reference fixtures.

### Modified Capabilities
- `script-tensor-io`: named tensors and the `emb.run` Lua surface support a `bool` dtype.
- `tokenizer`: the tokenizer exposes special-token IDs and an encode-without-special-tokens variant for sequence builders.

## Impact

- `internal/onnx/named.go`, `internal/script/host.go` — bool `TensorType` + Lua dtype.
- `internal/tokenizer/{tokenizer,reference}.go` — special-token IDs + plain encode (no schema change to `Encode(text, maxLength)`).
- `scripts/laya.lua` (new preset, mounted via `script_preload`), `test-laya.yaml` (new example), `testdata/laya/` vendored fixtures + parity test (pattern of `internal/script/gliner_test.go` / `internal/server/script_runtime_parity_test.go`).
- `README.md` — a "Laya decision models" section.
- `openspec/specs/script-tensor-io/spec.md` — structural repair of a pre-existing defect (delta headers leaked into the main spec by an earlier archive; same bug class `harden-0-4-0-release` fixed for `script-eval`/`lru-cache`).
- No wire-format change; no breaking changes. Downloads of the ~820MB fp16 checkpoints are operator-side (same `hfhub` path as other models).