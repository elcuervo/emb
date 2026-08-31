## 1. Server config: max_texts / max_pairs

- [x] 1.1 Add `MaxTexts int` and `MaxPairs int` to `Config` (`internal/config/config.go`) with YAML keys `max_texts`/`max_pairs`; add `-max-texts`/`-max-pairs` flags to `ParseFlags`; default 4096 when unset, 0 = unlimited (validate: negative rejected); verify config tests parse all forms (unset → 4096, explicit, 0, negative → error)
- [x] 1.2 Wire the values into `Server` (`internal/server/server.go` New/Options like `maxConcurrentReqs`); add `max_texts`/`max_pairs` entries to the `configParams` registry (`internal/server/config.go`) with GET/SET (non-integer or negative SET → error, value unchanged); verify a CONFIG test round-trips GET/SET and rejects invalid values

## 2. EMB truncation cap

- [x] 2.1 Implement truncation in `handleEMB`: when `maxTexts > 0` and `len(texts) > maxTexts`, process only the first `maxTexts` texts (cache lookup and inference scoped to the prefix) and reply with one slot per requested text — prefix embeddings, overflow `null` slots (single-text bulk reply unchanged); verify RESP-level tests: over-cap reply has N entries with null tail, within-cap unchanged, 0 = unlimited
- [x] 2.2 Verify truncation skips all work for overflow texts: no cache lookup/Set, no tokenization (assert via counters: `tokens` reflects only the prefix), no error to the client; verify a test asserts `EMB.INFO <model>` tokens stay ≤ prefix×max_length for an oversized command

## 3. EMB.MULTI truncation cap

- [x] 3.1 Implement truncation in `handleEMBMULTI` after the pairs-count check: when `maxPairs > 0` and `n > maxPairs`, fan out only the first `maxPairs` pairs and reply with one slot per requested pair (overflow `null`); verify RESP-level tests: over-cap reply has n entries with null tail, within-cap MGET semantics unchanged, 0 = unlimited
- [x] 3.2 Verify overflow pairs are skipped entirely (no `GetOrInit`, no `Pool.Embed`, no cache write); verify a test asserts `total_requests` increases by at most `maxPairs` and `truncated_pairs` counts the overflow

## 4. EMB.STATS counters

- [x] 4.1 Add `truncated_texts` and `truncated_pairs` fields to `EMB.STATS` (flat array fields near `active_requests`), incremented by overflow items on truncation; verify a stats test asserts the values after scripted truncations (e.g., max_texts 2 + 4-text EMB → `truncated_texts` 2; max_pairs 3 + 5-pair MULTI → `truncated_pairs` 2) and zero on a clean server

## 5. Gem chunking

- [x] 5.1 Add `batch_size` to `Emb::Configuration` OPTIONS (default 512, global + per-client override via `Emb.new(batch_size:)`); verify configuration specs cover default, global set, and per-client override
- [x] 5.2 Chunk `BATCH_BLOCK` (`gems/emb/lib/emb/batch.rb`): `client_items.each_slice(batch_size)` → one `EMB.MULTI` per slice, offset accounting per slice, `loader.call` per item preserving single/multi-text value shapes and MGET-null propagation; verify batch specs: small scope → 1 command, 250 pairs with batch_size 100 → 3 commands (100/100/50), order + nil preserved across chunks
- [x] 5.3 Chunk `MultiProxy#run` (`gems/emb/lib/emb/multi.rb`) with the same `batch_size`, reassembling the results array in collection order with nils for failures; verify the multi-model batch spec covers chunked reassembly

## 6. Docs + validation

- [x] 6.1 Document `max_texts`/`max_pairs` (server) and `batch_size` (gem) in `config.yaml` comments (per decision, README is not updated in this change); verify the comments describe the defaults, `0` = unlimited, and truncation-null behavior
- [x] 6.2 Full suite green: `go test ./internal/server/` + Ruby client specs (`just all` or targeted) pass; note the pre-existing `TestAsyncTokenizerOverlapsWork` flake separately if it fails
- [x] 6.3 Oversized-command validation test (`internal/server/`): reproduce the runaway signature — a single `EMB.MULTI` with ~100k near-max-length pairs (max_pairs 256) — and assert the server completes within a bounded wall-clock deadline while `total_requests` ≤ `max_pairs`, `tokens` ≤ `max_pairs × max_length`, reply has 100k slots with null tail, and `truncated_pairs` == 99744; also assert the same workload with `max_pairs 0` (or `max_texts 0`) exhibits the unbounded pre-change counters (the problem this change validates)
- [x] 6.4 End-to-end smoke: boot server with `CONFIG SET max_pairs 100`, send a 300-pair `EMB.MULTI` → 300-slot reply with 100 bulks + 200 nulls; with batch_size 100 gem client, 250 deferred pairs resolve correctly across 3 chunked commands; `EMB.STATS` shows `truncated_pairs` ≥ 1
- [x] 6.5 Per-model `max_length` truncation validation: same over-long text embeds at 64 tokens on a max_length-64 model and at 512 on a max_length-512 model, short texts untruncated (server-level test with two registered models)