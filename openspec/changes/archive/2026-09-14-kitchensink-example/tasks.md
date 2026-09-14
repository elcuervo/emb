## 1. Scaffold the example

- [x] 1.1 Create `examples/kitchensink/` and write `emb.yaml`: `listen` on a dedicated port (**16400**, so it never collides with `just all` on 16379 or the client suites on 16380–16384), `cache: auto`, and one `minilm` entry with `model_repo: Xenova/all-MiniLM-L6-v2`, `preload: true`, `normalize: true`, `max_length: 256`. Verify: `./bin/emb -config examples/kitchensink/emb.yaml` starts inside `nix develop` and `redis-cli -p 16400 EMB.MODELS` lists exactly one model.
- [x] 1.2 Add a commented Tier 2 block to `emb.yaml` (a `reranker` model preloading `../scripts/rerank.lua`, with `preload: true`). Verify: the file still loads with the block commented, and the block naming a nonexistent model does not affect startup.
- [x] 1.3 Write `run.sh`: start Redis on **6399** with append-only persistence rooted in a gitignored `examples/kitchensink/.state/`, start `emb` with `emb.yaml`, poll `EMB.READY` until `+OK` (bounded, failing loudly on timeout), and run the app with `EMB_URL`/`REDIS_URL` exported. The servers persist between invocations via pidfiles, and `run.sh stop` shuts them down. Verify: running it from a cold shell leaves no connection error in the app's first request; a second invocation reuses the running servers rather than restarting them; and `run.sh stop` leaves no orphaned `emb` or `redis-server` process.

## 2. The application

- [x] 2.1 Write `app.rb`'s chunker: split on blank lines, hard-wrap oversized paragraphs on word boundaries at ~900 characters, drop fragments under 80 characters, and derive each chunk's id as a digest of source path + text. Verify: two runs over the same file produce identical ids, and editing one paragraph changes exactly one id.
- [x] 2.2 Implement `index`: `DEL` the index key, then per slice of 512 chunks issue one `EMB.MULTI` and a pipelined `VADD` + `VSETATTR` per chunk, skipping any chunk whose embedding came back null and reporting the skipped count. Verify: indexing the repo's documentation reports one batch for the whole corpus and a final element count equal to `VCARD`.
- [x] 2.3 Implement `search`: embed the query with the same model, issue `VSIM <key> VALUES 384 <vector> COUNT <k> WITHSCORES WITHATTRIBS` (plus `FILTER` when given), and parse the flat reply in threes, treating a nil attributes field as empty. Verify: a query returns ranked hits carrying score, source, and text, and no second read is issued for the text.
- [x] 2.4 Implement `stats`: `EMB.stats`, `EMB.server_info('cache')`, and the index's `VCARD`/`VDIM`. Verify: the output includes the model line reporting `norm=true` and non-zero cache counters after a repeated search.
- [x] 2.5 Implement the CLI dispatch for `index`, `search`, `stats`, aborting non-zero with usage on an unknown operation, an `index` with no files, or a `search` with no query. Verify: all three failure invocations exit non-zero and print usage on stderr.
- [x] 2.6 Confirm the example adds no dependency. Verify: `app.rb` runs to completion using only `gems/emb` and its existing `redis-client`, with no addition to any gemspec, Gemfile, or `go.mod`.

## 3. Verify end to end

- [x] 3.1 Index `README.md`, `DESIGN.md`, and `website/PRODUCT.md`, then run three natural-language queries. Verify: each returns plausible, distinct top hits that a manual grep of the same files corroborates.
- [x] 3.2 Re-index the same corpus unchanged and compare element counts. Verify: the count after the second run equals the count after the first, and no element ids changed.
- [x] 3.3 Search the same query twice and read the counters. Verify: `cache_hits` increases on the second search and `total_requests` does not increase by two, proving the configuration's caching is in effect.
- [x] 3.4 Force a partial failure (index a slice containing an entry guaranteed to embed as null) and confirm the run continues. Verify: the remaining chunks in that slice are present in the index, the failed chunks are absent, the skipped count is reported, and no zero or placeholder vector exists in the index.
- [x] 3.5 Confirm the index's shape. Verify: the whole corpus — vectors and payloads — is reachable under one key, and `redis-cli -p 6399 KEYS '*'` shows no parallel payload structure.
- [x] 3.6 Validate retrieval against an independent reference rather than by eye: query with a stored chunk's own text and assert the top hit is that chunk carrying that chunk's text; then recompute cosine against every stored vector via `VEMB` and confirm `VSIM`'s ordering matches brute force and that its score equals `(1 + cos) / 2` within quantization error. Verify: all assertions hold. This is the check that caught the score being rescaled rather than a raw cosine, and it is the reason the README documents the mapping.

## 4. Document the example

- [x] 4.1 Write `examples/kitchensink/README.md`: what the two servers are, the run commands, what each half contributes, the configuration table with the reason for each load-bearing field, and the gotchas (index/query must share model and normalization; vector sets quantize by default; `index` rebuilds from scratch). Verify: the README states the Redis `vectorset` requirement and the port choices.
- [x] 4.2 Fill every number in the README from an actual run — element counts, scores, and cache counters — and state the command that produced each. Verify: re-running the stated command reproduces the stated number.
- [x] 4.3 Ensure the README advertises nothing unimplemented: the Tier 2 block stays a commented configuration with a one-line forward pointer and **no** worked `--rerank` example until the option exists. Verify: every operation and option named in the README is accepted by `app.rb` and executes.

## 5. Integrate with the repository

- [x] 5.1 Add a `just kitchensink` passthrough that runs the example's `run.sh` with pass-through arguments. Verify: `just --list` shows the target, and `just kitchensink search '"how does batching work"' 3` returns results from a cold start. (Arguments are forwarded through the shell, so an argument containing spaces needs its own quotes — documented in the target's comment.)
- [x] 5.2 Point the root `README.md` at the example from its `## Custom scripts` section, whose closing paragraph already sends readers to `examples/scripts/` — add one line distinguishing that directory (individual constructs) from `examples/kitchensink/` (the end-to-end runnable application). Verify: the link resolves and the sentence introduces no capability the example does not implement.

## 6. Validate

- [x] 6.1 Run the repository's standard checks and confirm nothing regressed: `just test`, `just lint`, and `just verify-harness`. Verify: all pass, and `git status` shows changes confined to `examples/kitchensink/`, `justfile`, `README.md`, `.gitignore`, and `openspec/`.
- [x] 6.2 Run `openspec validate kitchensink-example --strict` and `just deadcode`. Verify: the change validates and the deadcode gate is unaffected.
