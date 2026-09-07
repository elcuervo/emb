## 1. Compiled-script cache

- [x] 1.1 Add a compile cache to `internal/script`: `lua.CompileString` once → cached `*FunctionProto` per (model, SHA, source) fed through `NewFunctionFromProto` in each fresh state; `Eval` keeps a compile hook (or returns the compile count) so tests can observe reuse. Verify: unit test executes the same source twice and asserts the second run does not recompile (counter), with identical results
- [x] 1.2 Wire invalidation: `EMB.SCRIPT FLUSH` clears compiled protos with the source cache; `EMB.SCRIPT LOAD` recompiles. Verify: server tests — flush then EVSHA recompiles and still replies identically

## 2. Scripted session pool + config

- [x] 2.1 `ModelConfig` gains `script_workers` and `script_preload` keys (config + validation; 0/absent = auto). Verify: config tests parse both, reject negatives; `model-loading` delta scenarios hold
- [x] 2.2 Registry session pool: `ScriptResources` holds `script_workers` sessions created from the same factory (auto-tune by RAM like the embed pool, capped at GOMAXPROCS), round-robin selection in `RunNamed`; `Close` closes all. Verify: registry test asserts pool size for explicit workers; concurrent evals distribute across sessions (fake session counting distinct instances)
- [x] 2.3 `script_preload`: `LoadModel` warms `ScriptResources` at load when set. Verify: registry test — preload model has a ready session before any eval; `test-gliner.yaml` gains `script_preload: true`
- [x] 2.4 Server flow-through: `runScripted` uses the pooled path; `EMB.SCRIPT`/reply/cache behavior unchanged. Verify: full server suite still green (no reply-shape or cache-key changes)

## 3. Batched inference block

- [x] 3.1 `emb.run_batch(items)` host block: validate uniform tensor names/ranks/dtypes across items, pad each named tensor to per-dim max (zero-fill), concatenate along batch axis, one `RunNamed` call, split outputs back per item. Verify: unit tests — two items produce one session call (fake session call counter), outputs equal per-item `emb.run` results, mismatched names/dtypes error
- [x] 3.2 Server single-eval semantics: `runScripted` evaluates ONCE per request with ALL (miss) texts as KEYS; multi-text results must be an array 1:1 with the texts (else error); per-text elements are cached individually (hits skip the eval); reply shape unchanged (single → value, multi → array of values). Verify: server tests — two-text EVSHA runs the script once (compile/session call counting), unmatched array errors, cache still per-text; existing GLiNER wire multi-text test unchanged
- [x] 3.3 Update `examples/scripts/gliner2.lua` to batch all KEYS texts into one `emb.run_batch` and decode per item (single text = batch of one, returns the hash; multiple texts = array of hashes). Verify: GLiNER golden test unchanged; GLiNER wire multi-text test still returns per-text hashes
- [x] 3.4 Benchmarks: multi-text cells (8 and 16 × ~20-word texts) measuring serial vs batched. Verify: `just bench-gliner` reports batched 16-text ≥ 2x the serial sum on the real model (observed: 777ms serial -> 331ms batched; batch-8 is neutral, documented in design)

## 4. Sweep and docs

- [x] 4.1 `EMB.HELP`/specs/design parity for the new block and keys. Verify: help mentions `emb.run_batch`; `openspec validate` passes
- [x] 4.2 Full suite: `just test`, gem rspec, rubocop, golangci-lint, `just build` all green. Verify: CI-equivalent sweep passes with the before/after benchmark numbers recorded in the change