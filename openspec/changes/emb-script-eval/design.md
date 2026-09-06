# Design: emb-script-eval

## Context

`emb` serves one execution kind: BERT-shaped embedding through `Pool` (tokenizer → `Session.Run(input_ids, attention_mask)` → pooling → float32 bytes). The seam points: `tokenizer.Encode(text)` is single-string only; `onnx.Session.Run` takes exactly two named inputs and returns one pooled tensor; `internal/server` calls `entry.Pool.Embed` directly. Config `ModelConfig.Validate` only checks file existence, so a GLiNER-style model (7 input tensors, rank-4 `logits` output, no inferable dim) passes registration today — it just can't run through the embed path.

Goal: mount such models and run them via Redis-style scripts — `EMB.EVAL`/`EMB.EVSHA`/`EMB.SCRIPT` — with labels/params per request, no config changes, Redis-native reply constructs (hash etc.). See proposal.md for motivation and specs/script-eval for the behavior contract.

## Goals / Non-Goals

**Goals:**
- Execute sandboxed Lua (gopher-lua, Lua 5.1 semantics — Redis parity) against any config-mounted model, per request.
- Keep the BERT embedding path and its performance machinery completely untouched; additive plumbing only.
- Serve the `cuerbot/gliner2-multi-v1` testbed end-to-end with dynamic labels in ARGV and hash replies.
- Content-addressed caching of scripted replies through the existing LRU cache.

**Non-Goals (v1):**
- Cross-model composition inside scripts (`emb.embed` of another model) — deferred.
- Batching/timing-window execution for scripted runs — per-request sequential execution for v1.
- RESP3 map encoding — RESP2 flat field/value pairs only.
- Named (non-positional) ARGV parameters.
- Real hash *datatype* semantics (HSET/HGET on stored outputs) — reply-shape hashes only.

## Decisions

### D1: Engine — gopher-lua, fresh interpreter per evaluation

Redis ships Lua 5.1; gopher-lua implements 5.1 semantics in pure Go and gives us per-state isolation. **A new Lua state per evaluation** (not a shared interpreter like Redis's) means a runaway script fails only its own request — no global blocking, no `SCRIPT KILL` needed. Alternatives: starlark (hermetic by design, but not Redis-compatible reply semantics), goja (JS; heavier, sandboxing is manual), builtin Go adapters only (not what the user asked — no request-time scripts). Interpreter init cost is small; pool later only if profiling demands it.

Sandbox surface (per spec): open only a curated stdlib subset (`string`, `table`, `math` minus `random`); register host functions `emb.run`, `emb.tokenize.*` (`words`, `pretokenized`, `encode`, `encode_pair`), `emb.math` (`sigmoid`, `softmax`, `argmax`), `json`; omit `io`/`os`/`debug`/`package`. Budgets: wall-clock deadline via gopher-lua context support, recursive-depth bound via VM call-stack limits, and a script-size cap; exceeding any replies with an error for that request only. Scripts are pure compute → replies deterministic → cacheable (this is the safety contract that makes content-addressed caching correct — see D4).

### D2: Command shape — model-first, numtexts disambiguator

`EMB.EVAL <model> <script> <numtexts> <text...> <arg...>` — model first, matching every existing command (`EMB <model> <text>`, `EMB.MULTI`). `numtexts` plays Redis `numkeys`'s role: it fixes where texts end and args begin. Script cache is **per model** (mirrors the registry; same SHA against two models = two entries). Alternative considered: Redis-pure placement of the model inside KEYS (`KEYS[1]=model`) — more faithful to EVAL but breaks the model-first convention every other command uses and complicates cache scoping; rejected.

### D3: Scripted execution path — one dedicated session per model, mutex-serialized

The registry gains a lazy scripted session for a model, created from the *same session factory/runtime config* the pool uses (`onnx.NewRuntimeSessionFromBytes`, input names from the graph). Since ORT sessions serialize runs, a single session guarded by a mutex serves scripted evals. The new `onnx.RunNamed` API (named int64/float32 tensors in → named outputs out) is **additive**: `Session.Run` and the zero-copy `RuntimeSession` stay exactly as they are for the BERT path. Input tensor dtype/shape validation happens in the host function wrapper, so a script can't feed malformed tensors past ORT's own checks.

### D4: Content-addressed cache — model:sha:args:text

Reuses the existing `Cache` (bytes) with key `model ‖ sha1(script) ‖ sha1(args joined with NUL) ‖ text`. Per-text entries, array assembly on the server side exactly like `handleEMB`'s hit/miss loop. Because the sandbox forbids `math.random`/time and there is no state mutation, same key ⇒ same bytes, so caching is correct by construction. Distinct label sets are distinct keys automatically.

### D5: Lua→RESP2 conversion — hash is just the table grammar

A recursive converter (`script.Convert(writer, luaValue)`) implements the spec grammar: string → bulk (byte-safe — the float32 embedding path is a Lua string of raw bytes, no special API), integral number → integer, non-integral number → bulk string via `strconv.FormatFloat` (RESP2 has no double; avoids Redis's silent truncation), list table → array, string-keyed table → **hash** as flat field/value pairs (HGETALL shape — RESP2 has no map type; a future RESP3 path changes only the wire encoding, not the Lua-side rule), `nil`/`false` → null, `{err=…}` → error via `WriteError`.

### D6: Testbed — example script built on shared blocks + golden fixtures

The server maintains only generic building blocks; model glue stays in user scripts (an explicit non-goal — no maintained GLiNER adapter). `examples/scripts/gliner2.lua` demonstrates the pattern, mirroring `cuerbot`'s `decoder.py` semantics (schema layout `( [P] prompt ( [E] LAB…) ) [SEP_TEXT] words`, label-position indexing, span search over word-start positions, sigmoid threshold, best-span + overlap suppression) using the blocks: `emb.tokenize.words` (BertPreTokenizer-equivalent split with byte offsets, implemented rune-correctly as `tokenizer.SplitWords`), `emb.tokenize.pretokenized`, and `emb.run`. Correctness: a golden-file test (`gliner_test.go`, `-update` flag) runs the real `model_int8.onnx` through the script and snapshots (text, labels) → entities; the repo's Python fixture generator requires torch, so goldens are generated from the model itself in Go. Testbed config mounts `cuerbot/gliner2-multi-v1` with an explicit `onnx:` pointing at `model_int8.onnx` (376MB vs 1.2GB fp32); download is gated behind `just download-gliner-model`, not the default `just test` suite.

### D7: Registry/server plumbing stays narrow

New handlers (`handleEMBEVAL`, `handleEVSHA`, `handleSCRIPT`) join the existing mux; they share `handleEMB`'s guards (shutdown check, auth, `active` wait-group) and join the `max_concurrent_requests` gate like `emb`/`emb.multi`. `EMB.HELP` documents the family and the reply grammar. The batch middleware in `gems/emb` is untouched; `eval`/`evalsha`/`script.load`/`script.exists`/`script.flush` client methods return raw replies plus a typed view (flat pair array → Ruby Hash) for hash replies.

## Risks / Trade-offs

- **`daulet/tokenizers` may not expose word-level (`is_split_into_words`) encoding** → Fallback is mechanical and already understood: encode each word separately with `add_special_tokens=false` and merge, tracking the word index per subword ourselves. Either way `EncodePretokenized` returns `(ids, word_ids)`.
- **gopher-lua deadline enforcement granularity** (context checked between VM steps, not inside tight loops) → Mitigate with the call-stack depth limit, script-size cap, and tests with pathological scripts (infinite loops, deep recursion); a budget error must reply per-request without affecting other evals (covered by the integration test).
- **Plain `EMB` on a scripted model** (e.g. someone sends `EMB gliner …`) attempts the embed path and fails (dim=0 / rank-4 output). → Acceptable for v1: error is contained; document that non-embedding models are consumed via `EMB.EVSHA` only. A config `script:` marker key (to refuse/route) is a later, additive refinement.
- **`EMB.MODELS`/`EMB.INFO` dim is meaningless for scripted models** (would report the last logits dim or 0) → Cosmetic for v1; scripted models are identified by the reply grammar/script, not a catalog type marker (deliberately — see spec "script is the contract"). A type field is a clean future addition.
- **1.2GB fp32 download for testbed** → Use explicit `onnx:` → `model_int8.onnx` (376MB); hfhub already resolves `model.onnx` directly at repo root with zero code changes.
- **Sequential per-request execution (no batcher) for scripted runs** → Matches GLiNER-class low-QPS usage; within-request batching of same-schema texts is a future optimization (all 7 inputs have dynamic batch dims, so it's viable).

## Migration Plan

Pure additive — no existing command, config key, or reply shape changes. Deploy order: (1) `internal/onnx` `RunNamed` + `internal/tokenizer` `EncodePretokenized`/`SplitWords` (all additive, unit-tested); (2) `internal/script` engine/sandbox/conversion + host blocks (`emb.tokenize.*`, `emb.math`, `emb.run`) + cache keys; (3) server command family + gating; (4) example scripts (`gliner2.lua`, then classification/QA/reranker) + golden/wire fixture tests; (5) `gems/emb` client methods. Rollback: revert commits; existing deployments unaffected since no behavior changes unless the new commands are used.

## Baseline block set (D7)

Extension decision — the host surface is the shared baseline for writing scripts across model families; nothing model-specific is maintained. Verified against real exports: distilbert sst-2 (`input_ids, attention_mask → logits [-1,2]`), distilbert squad (`→ start_logits, end_logits`), bge-reranker-base (`→ logits [-1,1]`), gliner2 (7 int64 inputs, `logits [1,seq,w,n]`).

| Host block | Used by | Rationale |
|---|---|---|
| `emb.run(inputs) → outputs` | all | named tensors in/out; returns every graph output (QA needs both start+end logits) |
| `emb.tokenize.words(text)` | span models (GLiNER) | BertPreTokenizer-equivalent split, byte offsets |
| `emb.tokenize.pretokenized(words, n)` | span models | word-aligned encode with word_ids (is_split_into_words semantics) |
| `emb.tokenize.encode(text, n)` | classification, QA, token-NER, rerankers | the model's OWN pretokenizer — correctness-critical: byte-level-BPE tokenizers (XLM-R family) split differently than BertPreTokenizer, so `words`-based input silently degrades them |
| `emb.tokenize.encode_pair(a, b, n)` | cross-encoders, QA | BERT-family `[CLS] a [SEP] b [SEP]` template; returns `sep` position + per-part offsets so QA spans slice the original context without a decode block |
| `emb.math.sigmoid` | rerankers, span scoring | vectorized (array in → array out) so hot per-candidate loops cross the Go/Lua boundary once per label batch, not per candidate; scalar accepted for convenience |
| `emb.math.softmax` / `argmax` | classification, token-NER | stable softmax (subtract max) and first-maximum argmax — deterministic, O(n) per request |
| `json` | structured glue | (existing) |

Design notes:
- **Sigmoid in the math lib (user decision)**: even though the naive form is one Lua line, a shared `emb.math` gives every script the same primitives with defined semantics; the vectorized form sidesteps the per-element boundary cost that argued for leaving it user-space.
- **encode_pair template assumption**: the pair template is BERT-family ([CLS]/[SEP] composition). Models with other pair formats stay expressible via `encode` + explicit separator ids (the script composes the template itself).
- **Offsets are byte-based** (matching the tokenizers binding, verified by the multi-byte `EncodePretokenized` tests); scripts slice original strings with `string.sub(text, s, e-1)`.
- **Seq2seq (T5/BART)**: expressible via repeated `emb.run` within one evaluation (the session persists across host calls) but without KV-cache access it recomputes the encoder per step — OK for short outputs, not a supported fast path. Documented, not blocked.
- **Multi-text batching** inside scripts (stacks of same-schema inputs) remains user-space padding in v1.

## Open Questions

(Deferrable without affecting the specs, approach, or task breakdown.)

- Script-size cap default (64KB like Redis's `lua-time-limit` spirit?) and whether it's configurable.
- Whether `EMB.SCRIPT LIST` (enumerate cached SHAs per model) is worth adding alongside EXISTS/FLUSH.
- Named ARGV convention (e.g. `name=value` pairs or a single JSON blob) for scripts with >3 parameters — positional suffices for the testbed.