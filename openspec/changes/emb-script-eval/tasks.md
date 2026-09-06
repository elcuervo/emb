## 1. Foundational plumbing (additive, BERT path untouched)

- [x] 1.1 Add `onnx.RunNamed`: run a session with named int64/float32 input tensors (shape+data) and return named outputs as `{Shape []int64, Data}`; keep `Session.Run`/zero-copy `RuntimeSession` unchanged. Verify: unit test runs a real session through `RunNamed` with multiple named inputs and asserts output values; existing onnx tests still pass
- [x] 1.2 Add `tokenizer.EncodePretokenized(words []string, maxLen int) (ids []int64, wordIDs []int64|nil, err error)`: word-aligned encode with no special tokens; if the underlying lib lacks `is_split_into_words`, fall back to per-word encodes merged with word-index bookkeeping. Verify: unit test encodes words incl. multi-subword words (e.g. "tokenization" → several subwords) and asserts each subword maps to the right word index and no special tokens appear
- [x] 1.3 Add `github.com/yuin/gopher-lua` dependency. Verify: `go mod tidy` clean and `nix develop --command bash -c 'go build ./...'` succeeds

## 2. internal/script package: engine, sandbox, conversion, cache

- [x] 2.1 Engine wrapper: fresh `LState` per evaluation, curated stdlib subset (string/table/math minus `random`/bit), host-function registration point, KEYS (texts) and ARGV (args) globals. Verify: unit test executes a trivial script reading KEYS/ARGV and returns the expected Lua value; `os`/`io`/`debug` libraries are absent
- [x] 2.2 Budgets: script-size cap (default 64KB), wall-clock deadline via gopher-lua context support, call-stack depth limit. Verify: unit tests — infinite-loop script errors after deadline; deep-recursion script errors on stack limit; oversized script rejected before compile; each failure is per-request (no leaked state)
- [x] 2.3 Lua→RESP2 conversion (`script.Convert`): string→bulk, integral number→integer, non-integral number→bulk string, list table→array, string-keyed table→hash as flat field/value pairs, nil/false→null, `{err=…}`→error, recursive nesting. Verify: table-driven unit test against a capture writer asserting exact RESP bytes for each shape incl. a nested hash-inside-hash

## 3. Host functions

- [x] 3.1 `emb.run(inputs) → outputs`: wraps `onnx.RunNamed` against the evaluation's bound model session; validates tensor names/dtypes/shapes before handing to ORT. Verify: unit test runs a fixture model (or fake session) through a script that builds named inputs and reads named outputs back into Lua tables
- [x] 3.2 `emb.tokenize.pretokenized(words) → (ids, word_ids)` and `json` (encode/decode). Verify: unit tests — pretokenized host fn round-trips a word list with alignment; json encodes/decodes Lua tables both ways

## 4. Server command family

- [x] 4.1 Registry: lazy per-model scripted session created from the same session factory as the pool, mutex-serialized. Verify: registry test asserts one session per model, created on first scripted eval, and runs serialize
- [x] 4.2 Script cache per model: `EMB.SCRIPT LOAD` (compile + cache + return SHA1, error on bad script), `EMB.SCRIPT EXISTS` (1/0 flags per model), `EMB.SCRIPT FLUSH [model]`. Verify: server tests cover load→exists→flush and the per-model isolation (loaded for A, absent for B)
- [x] 4.3 `EMB.EVAL`/`EMB.EVSHA` handlers: model-first parsing with `numtexts` disambiguator, unknown-model and unknown-SHA errors, wrong-arity errors, shared guards (shutdown, auth, `active` wait-group), and inclusion in the `max_concurrent_requests` gate. Verify: server tests assert each error path and that a busy gate rejects `EMB.EVSHA` like `EMB`
- [x] 4.4 Multi-text replies: array of per-text converted values; per-text content-addressed cache hit/miss assembly like `handleEMB` (bulk(s) for embed, converted values for scripted). Verify: server test with 2 texts and mixed cache hit/miss asserts reply shape and cache keys
- [x] 4.5 `EMB.HELP` documents the family and reply grammar. Verify: help output contains EVAL/EVSHA/SCRIPT lines and the hash-conversion note

## 5. GLiNER testbed

- [x] 5.1 Example script `examples/scripts/gliner2.lua` demonstrating the building blocks (no maintained adapter): schema layout `( [P] prompt ( [E] LAB…) ) [SEP_TEXT] words`, label-position indexing, 7-tensor construction via `emb.tokenize.words`/`emb.tokenize.pretokenized`/`emb.run`, span search over word-start positions, sigmoid threshold, best-span selection, overlap suppression, entities hash return. Verify: script loads in the engine and returns hash-shaped replies for the Apple sentence with PERSON/ORG/PRODUCT
- [x] 5.2 Golden fixtures: golden-file test (`gliner_test.go`, `-update` flag) snapshots (text, labels) → entities generated from the real `model_int8.onnx`; the repo's Python fixture generator needs torch and is not run in CI. Verify: `go test ./internal/script/ -run GLiNER` drives the Lua path over the golden cases and matches entity output exactly
- [x] 5.3 Testbed config + smoke: config mounting `cuerbot/gliner2-multi-v1` with explicit `onnx:` → `model_int8.onnx`; gated manual/CI step that reproduces the spec scenarios over the wire (extract entities, different labels, two texts → array of hashes). Verify: recorded transcript matches spec scenarios; step is excluded from default `just test`

## 6. Ruby client

- [x] 6.1 Client methods: `eval(model, script, texts, args)`, `evalsha(...)`, `script.load`, `script.exists`, `script.flush`. Verify: gem specs assert command bytes and replies; typed decode turns flat-pair hash replies into a Ruby Hash while embed batch decoding is untouched
- [x] 6.2 Docs: `EMB.HELP` parity + client README for the script surface. Verify: docs build/pass lint (`bundle exec rubocop`)

## 7. Verification sweep

- [x] 7.1 `just test` (server + gems), `just lint`, `just build` all green. Verify: full CI-equivalent suite passes
- [x] 7.2 Spec walkthrough: end-to-end test covering load→evalsha, unknown sha, per-model exists, distinct label sets as distinct cache entries, and budget errors replying per-request. Verify: integration test (or recorded transcript) matches every scenario in specs/script-eval

## 8. Baseline blocks for more model families

- [x] 8.1 `emb.math` host functions: `sigmoid` (number or array in/array out, vectorized), `softmax` (stable: subtract max), `argmax` (1-based index + value of first maximum). Verify: unit tests — vector sigmoid values, stable softmax on large inputs (no overflow), argmax tie-break to first, empty/non-numeric arrays error
- [x] 8.2 `emb.tokenize.encode(text, maxLen)` host block → `{ids, mask, offsets}` using the model tokenizer's own pipeline. Verify: unit test — ids/mask equal the embedding path's `Encode` values for the same text (minilm fixture), offsets slice the original text (multibyte case), truncation applied
- [x] 8.3 `emb.tokenize.encode_pair(a, b, maxLen)` host block → composes `[CLS] a [SEP] b [SEP]`, returns `{ids, mask, offsets, sep}` with per-part offsets (tokens of `b` slice `b` directly). Verify: unit test — ids equal CLS+encode(a)+SEP+encode(b)+SEP, `sep` points at the inter-part separator, offsets slice each part (incl. multibyte)
- [x] 8.4 Refactor `examples/scripts/gliner2.lua` decode to the baseline: score every candidate with a single vectorized `emb.math.sigmoid` call per label instead of the inline per-element form. Verify: GLiNER golden test still passes unchanged (`go test ./internal/script/ -run GLiNER`)
- [x] 8.5 Example scripts for the new families using the baseline: `sst2.lua` (classification), `qa.lua` (extractive QA with offset slicing), `rerank.lua` (cross-encoder with sigmoid). Verify: each loads in the engine and, with a fake session returning canned logits, produces the expected hash replies (evaluate-with-fake test)
- [x] 8.6 Docs parity: `EMB.HELP` mentions the math + tokenization blocks; design/spec reflect the baseline. Verify: help output contains `emb.math`; `openspec validate` passes, full suite green (`just test`, gem rspec, rubocop, golangci-lint)