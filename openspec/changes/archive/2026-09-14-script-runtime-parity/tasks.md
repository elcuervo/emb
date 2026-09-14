## 1. Session bound and explicit override

- [x] 1.1 Add a single source of truth for a model's effective embedding session count (1 for a batcher pool, configured workers, or auto-tune otherwise), derived from the already-defaulted model config — verify: a unit test asserts the helper returns 1 for a batching-enabled config, the configured value for `workers: 2`, and the auto-tuned value when both are unset.
- [x] 1.2 Use that helper for the auto-tuned `script_workers` default in `openScriptResources` — verify: with batching enabled (default) and `script_workers` unset, a script calling `emb.run` opens exactly one named-tensor session, observable through `EMB.INFO`.
- [x] 1.3 Stop clamping an explicitly configured `script_workers`; only the auto-tuned default is bounded — verify: a test with `script_workers: 4` and a batcher pool opens exactly four sessions.
- [x] 1.4 Add a test asserting script sessions and embedding pool sessions agree for both a batcher pool (1/1) and a worker pool (`workers: 2` → 2/2) — verify: the test passes for both configurations.
- [x] 1.5 Re-measure the default-config RSS for a raw-tensor script — verify: after `EMB` loads the pool, the first `emb.run` script opens at most one named-tensor session and grows RSS by at most one model footprint (was +973 MB / ~10 sessions), recorded in the benchmark baseline.

## 2. Image cache sharing

- [x] 2.1 Extract `Server.embedImages(entry, model, images)` owning the cache-read → preprocess/infer → cache-write sequence, with the same batch semantics as `handleIMG` — verify: the existing `EMB.IMG` tests pass unchanged against the refactor.
- [x] 2.2 Route `handleIMG` through the helper with no behaviour change — verify: `EMB.IMG` replies are byte-identical and the image cache stats move as before.
- [x] 2.3 Route `emb.image.embed` through the same helper — verify: with caching enabled, `EMB.IMG` followed by `emb.image.embed` on the same bytes performs no additional inference (fake-session call count stays 1).
- [x] 2.4 Verify the reverse direction — verify: a test asserts `EMB.IMG` after a scripted `emb.image.embed` is a cache hit.
- [x] 2.5 Verify repeated scripted image embeddings infer once — verify: two identical `emb.image.embed` calls with caching enabled invoke the image branch once.
- [x] 2.6 Verify caching-disabled behaviour is unchanged — verify: without a cache, `emb.image.embed` matches `EMB.IMG` for the same bytes.
- [x] 2.7 Confirm image cache entries stay bounded — verify: the helper writes through the same LRU budget as `EMB.IMG`, and a test asserts the cache size does not exceed its configured maximum across many distinct images.

## 3. Lazy image resources

- [x] 3.1 Bind `emb.image.*` through a `sync.Once`-guarded resolver instead of calling `entry.ImageResources()` before evaluating — verify: a script that never calls `emb.image.*` on an image-configured model leaves the model's image sessions unopened.
- [x] 3.2 Ensure `emb.image.info` still reports the configured plan without creating sessions — verify: an `emb.image.info` call succeeds and the reported session count stays 0.
- [x] 3.3 Verify a constant script on an image-configured model allocates nothing — verify: `EMB.INFO` reports zero script sessions and zero image sessions after such an evaluation.

## 4. Leak guarantees

- [x] 4.1 Define and document the ownership table (resource, owner, release point, bound) in the code comments of the owning types — verify: `ModelEntry`, `NamedRuntimeSession` and `scriptCache` each state their owner and bound.
- [x] 4.2 Ensure the output-tensor cache destroys evicted tensors and everything on `Close` — verify: a test evicts an entry and asserts the tensors are destroyed; `Close` with a populated cache is idempotent.
- [x] 4.3 Ensure a partially-constructed scripted session pool closes the sessions it already opened — verify: a test injects a session-creation failure partway and asserts no sessions remain open and the error surfaces.
- [x] 4.4 Ensure a failing evaluation releases what it allocated (deadline, budget, unknown output, post-`emb.run` script error) — verify: a table-driven test runs each failure mode and asserts the session's retained output-set count returns to its pre-call value.
- [x] 4.5 Add a sustained-traffic leak gate: a few thousand scripted evaluations across persistent connections leave goroutines at baseline and heap growth flat after GC — verify: the test uses the project's heap/goroutine probes and fails on growth beyond tolerance, with no `time.Sleep` assertions.
- [x] 4.6 Add a lifecycle leak gate: creating, exercising and closing a scripted model repeatedly does not accumulate sessions, tensors or tokenizers — verify: the test asserts the pre-cycle baseline is restored after each cycle.
- [x] 4.7 Verify the shared tokenizer is released exactly once when a model serves both `EMB` and scripts — verify: a test closes the registry and asserts a single close of the shared tokenizer (via a counting fake).
- [x] 4.8 Verify script source and bytecode caches stay bounded under many distinct scripts — verify: loading more distinct scripts than the cap leaves the cache at its cap.

## 5. Metal-parity and throughput benchmarks

- [x] 5.1 Add a bare-graph baseline benchmark: a script that runs the graph and returns without reducing its output, versus the same graph run directly — verify: the benchmark reports both sides and the derived ratio.
- [x] 5.2 Tighten the embedding-class parity budget to ≤1.30× at 8 tokens and ≤1.15× at 32/128 tokens, and assert it — verify: `just bench-budgets` passes at all three lengths on the reference machine.
- [x] 5.3 Assert the metal-parity overhead-share budget: bare scripted run ≤1.20× a direct run, and packed-read-plus-reduction ≤0.35× over the bare run — verify: the budget test passes and logs both ratios.
- [x] 5.4 Add a throughput-scaling benchmark and budget: with N script sessions, sustained throughput ≥0.85 × N × single-session throughput up to the core count — verify: the benchmark runs at N=1 and N=4; the recorded baseline ratio is the regression gate and the absolute 0.85 target is asserted with `EMB_BENCH_REFERENCE=1` on a quiet host.
- [x] 5.5 Extend the memory budget test to the batcher-pool default configuration (not just `script_workers: 1`) — verify: RSS growth for a raw-tensor script stays within one model footprint with batching enabled and script sessions do not exceed the pool's.
- [x] 5.6 Capture the new baselines and wire every new benchmark into `just bench` / `just bench-budgets` — verify: `just bench-script` and `just bench-budgets` run them and the baseline file records their values.

## 6. Documentation and consistency

- [x] 6.1 Document the `script_workers` override semantics and the memory/parallelism trade-off — verify: `README.md` states that unset is bounded by the embedding pool's session count and that an explicit value is honoured verbatim, with a sessions-vs-memory table.
- [x] 6.2 Update the production scripting notes for image cache sharing and lazy image resources — verify: the guide states that `emb.image.embed` shares the `EMB.IMG` cache and that image resources are opened on first image use.
- [x] 6.3 Reconcile `design.md` of the archived change with the new semantics — verify: no archived design text claims an explicit `script_workers` is clamped or that image resources open eagerly.
- [x] 6.4 Run the full standard checks — verify: `go test ./...`, `go vet ./...`, `golangci-lint run ./...` and `just build` all pass, and `openspec validate script-runtime-parity --strict` is valid.
