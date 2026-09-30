# Design

## Context

See proposal.md — Why. The deciding constraint set, from the council review:

- The Laya ONNX graph (per `tools/export_onnx.py` and the export repo) declares five inputs — `input_ids`, `attention_mask` (int64 `[b, seq]`), `marker_pos` (int64 `[b, markers]`), `marker_mask` (**bool** `[b, markers]`), `qtype` (int64 `[b]`) — and three outputs, `logits` `[b, markers]`, `act_logits` `[b, 2]`, `last_hidden_state` `[b, seq, dim]`.
- emb already serves non-embedding graphs through the script host: `internal/onnx/named.go` (`NamedRuntimeSession`, all graph inputs/outputs bound at session creation), `internal/script/host.go` (`emb.run` with per-run `outputs = {...}` selection and constant `fill = 0` tensors — proven by `website/repl/presets/zeroshot.lua`), `EMB.EVAL`/`EMB.EVSHA` with a content-addressed reply cache that already includes ARGV (`internal/script/cache.go`).
- `yalue/onnxruntime_go@v1.31.0` already builds `Tensor[bool]`; the bool gap is emb-side only (`NamedTensor` + the Lua dtype whitelist). `daulet/tokenizers@v1.27.0` has `Encode(str, addSpecialTokens bool)` but no `token_to_id`.
- The Ruby gem's parity fixtures exist and are small: `test/fixtures/tiny/*` (32-hidden model, `onnx_config.json`, `rl_agent_config.json` with out-of-range temperatures, `expected.json` answer corpus).

## Goals / Non-Goals

**Goals:**
- Reproduce the Ruby gem's sequence construction, calibration, and answer payloads with fixture-pinned parity.
- Reuse the scripted-model path entirely: no new Redis command, no new cache namespace, no batching changes.
- Keep the Go diff small and generic: the two enabling changes (bool tensors, tokenizer primitives) are reused by any future multi-input graph.

**Non-Goals:**
- No `EMB.PREDICT` native command (tipping condition documented in the proposal; a wrapper can follow later without changing this design).
- No embedding-only serving (`last_hidden_state` mean-pooling) in this change; the graph's encoder output already exists for it, and ORT pruning means a later decide/embed split is a per-run output selection only.
- No router/language heuristics, no fine-tuning, no checkpoint download automation beyond operator-facing example docs.

## Decisions

### D1. Serve over the existing script path; do not add a command

`EMB.EVAL <model> <laya> 1 <state> <questions-json> [<config-json>]` — state as `KEYS[1]` (a string; structured states arrive already Python-style-serialized), questions JSON as `ARGV[1]`, the checkpoint's budgets/temperatures as optional `ARGV[2]`. Since that envelope is a property of the checkpoint rather than the request, the `script-config` change moves it onto the model entry's `scripts` config and exposes it as `emb.script.config`, leaving `ARGV[2]` as a per-call override and the bare call `EMB.EVSHA laya <sha> 1 <state> <questions>`; a later task preset (`scripts/snake.lua`, see `laya-live-loop`) is the first consumer that also owns a loop. Rationale: the reply cache already content-addresses ARGV (distinct question sets and configs already miss/hit correctly), per-text caching is exactly right, and the preload/validation lifecycle is already specified. Alternative considered (rejected): a Go `EMB.PREDICT` — adds command, cache-key, reply-format, and validation surface for one client today; the council's tipping condition (≥2 independent clients needing a stable typed contract) is not met.

### D2. Bool dtype: extend `NamedTensor`, not a second session type

Add `TensorBool` to `internal/onnx/named.go` (reuse `ort.Tensor[bool]`, 1-byte packing on the Lua boundary), and `b1`/`bool` to the Lua dtype whitelist in `internal/script/host.go` (`namedTensorFromLua`/`namedTensorFromValue`). Rationale: one mechanism covers any future bool-input graph; the embedding `RuntimeSession` is untouched. Alternative (rejected): special-casing Laya in `runtime.go` — duplicates the named-session machinery.

### D3. Tokenizer primitives parsed, not probed

Expose `SpecialTokenIDs()` (mask/cls/sep/pad + the mask token's text) and `EncodePlain(text, maxLen)` (no special tokens) on the tokenizer interface, surfaced to scripts as `emb.tokenize.special_ids` / `emb.tokenize.encode_plain` (Hosts gains `EncodePlainIDs`/`SpecialTokenIDs`, named distinctly from the pre-existing offset `EncodePlain` host field). Discovery order: tokenizer-config special-token names → `tokenizer.json` `added_tokens` entries → single-token encode probes (mirrors the gem's `CANDIDATES` fallback). Rationale: `added_tokens` is exact for ModernBERT/mmBERT files (verified in the tiny fixture: `[PAD] 0/[UNK] 1/[CLS] 2/[SEP] 3/[MASK] 4`, `normalized: false`); probes are the fallback the gem itself uses. `daulet` divergence is a crate-version concern, pinned by the parity corpus.

Two more host additions fell out of the criteria/key-order problem and are part of this slice: `json.decode_ordered` (objects decode as `{k, v}` pair arrays in document order, because gopher-lua string-key iteration is Go-map-random and the VM cannot hold nil) and the `b1` dtype for `marker_mask`. Criteria label ORDER is the contract — marker order maps to label order — so the questions envelope is read with `decode_ordered`, and the `json.null` sentinel counts as a blank criterion.

### D4. `laya.lua` is the single decision implementation

All Laya logic — question validation, `build_sequence`, marker accounting, temperature buckets, softmax/confidence, answer assembly — lives in the preloaded Lua preset (`scripts/laya.lua`, mounted by `test-laya.yaml` with `script_preload`), exactly as GLiNER's sequence construction lives in its script. The script is the versioned asset; a future `EMB.PREDICT` is a transliteration against the same parity corpus. Rationale (council): a Go port buys typed errors and in-Lua unit granularity, not behavioral pinning — the corpus pins either equally; the script is testable via the existing RESP round-trip harness. Note the corpus is generated by upstream Python `laya` (not the Ruby port), so `laya.lua` tracks Python's `build_sequence`/`render_options` semantics — strings pass through unquoted, structured criteria become Python-style JSON, and the upstream `tokenizers` crate splits punctuation in Whitespace pretokenization (verified identical in `daulet`).

### D5. Feed everything, request what's needed

One session, all five inputs fed (constant `fill` tensors for unused branches — the zeroshot pattern), `outputs = {"logits", "act_logits"}` on the decide path. Rationale: avoids depending on ORT pruning of un-fed inputs; matches how `registry.go openScriptResources` already builds sessions (all inputs/outputs bound at creation). `last_hidden_state` is pre-head in the export, so a later embedding path can reuse the same script with different outputs and dummy markers — verified in `tools/export_onnx.py` (returned before `type_emb`/head math).

### D6. Parity harness = vendored fixtures + live RESP

Vendor ruby-laya's `test/fixtures/tiny/*` into `testdata/laya/` and pin via the existing `serveScriptModel` + `EMB.EVSHA` round-trip pattern (the `gliner_test.go` golden style, with a `-update` flag). Every corpus case (single-option choice, 12-option choice, `choice:11+` 0.1006 clamp, noul confidence, long-state truncation) asserts the full answer payload including `action.act_probability` and `input_tokens`. Real-checkpoint tokenizer parity is a separate operator-run golden regeneration (documented, not CI-default).

## Risks / Trade-offs

- [Tokenizer crate-version skew between `daulet` v1.27.0 and the Ruby gem's tokenizers binding could diverge on real ModernBERT/mmBERT files] → Tiny-fixture corpus pins the mechanism; a documented operator-run golden regeneration against a downloaded real checkpoint catches skew before any production use.
- [Lua floating-point vs Ruby `round(4)`] → The fixtures assert rounded values; if a boundary case drifts, the preset pins explicit `math.floor(x*10000+0.5)/10000`-style rounding like the gem's `.round(4)` semantics.
- [Questions JSON payload size vs `max_command_bytes`] → One arg holds the envelope; oversized commands are already rejected before decode by the request-size guardrail; document the bound.
- [Errors are `ERR <string>` from Lua, not gem-parity `ArgumentError` classes] → Accepted: the corpus only pins valid requests; validation errors are a documented "first-class `EMB.PREDICT`" tipping item, not a correctness gap.
- [Two decision implementations alive (gem + server) drift] → The vendored corpus is versioned with `laya_version` (`0.3.7`); bumping the fixture set is a one-task update.

## Migration Plan

Config-only: operators add a `models:` entry pointing at a downloaded export (`onnx:`/`tokenizer:`/`script_preload`) and preload `scripts/laya.lua`. No server default changes; the preset is inert unless mounted. Rollback = remove the config entry.

## Open Questions

None that block the design. The native `EMB.PREDICT` command and embedding-only serving are deliberately deferred (proposal Non-Goals) and can be added without changing the specs, the script, or the parity harness.