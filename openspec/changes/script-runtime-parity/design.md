# Design: scripted runtime parity, bounds, and leak safety

## Context

See `proposal.md` for the observed defects and their measurements. The constraints that shape the fixes:

- **Batching is on by default** (`registry.go` defaults `batching.timeout` to 1 ms), so a model's embedding pool is a *batcher* holding exactly one ORT session; `Pool.Stats().NumWorkers` reports `1` for it. A worker pool (`batching.timeout: 0`) holds `len(workers)` sessions.
- `openScriptResources` computed its bound from `cfg.Workers` (which is unset → `autoTuneWorkers` → cores), never comparing against what the embedding path actually holds. On a 24 GB / 10-core host that is 10 script sessions against a 1-session pool.
- `handleIMG` owns image caching via `imageCacheKey`; the `emb.image.embed` closure added by the previous change bypasses it entirely.
- `runScripted` calls `entry.ImageResources()` before evaluating, which triggers `imageOnce` for any model with an `image:` block.
- New ownership introduced by the previous change has no explicit contract: the per-session output-tensor cache (`outCache`), the tokenizer now shared between pool and scripts (`sharedTok`), and lazily created session/tokenizer handles.

## Goals / Non-Goals

**Goals**

- Scripted sessions never exceed the embedding path's real session count by default, in every configuration.
- `script_workers` becomes a real operator override rather than a silently clamped hint.
- Scripted and native image embeddings share one cache.
- Nothing image-related is allocated for scripts that do not use the image surface.
- Every scripted allocation has a single owner, a bound, and a leak test.
- Scripted overhead is quantified against the inference it wraps ("metal parity"), not only against a sibling command.

**Non-Goals**

- Cross-request batching of arbitrary named-tensor evaluations (unchanged non-goal).
- Eliminating the ORT→Go output copy (`RunNamed` returns Go tensors; a zero-copy contract is a larger API change).
- Changing the Lua surface, reply grammar, or script cache keys.
- A native `EMB.DISTANCE` command.

## Decisions

### 1. The script session bound is derived from the pool's *effective* session count

Add a single source of truth for "how many inference sessions does this model's embedding path hold", derived from the already-defaulted model config rather than from a loaded pool (so computing the bound does not force a pool load):

```
effectiveEmbeddingSessions(cfg) =
    if cfg.Batching.Timeout != nil && *cfg.Batching.Timeout > 0 -> 1   // batcher
    else if cfg.Workers > 0                                        -> cfg.Workers
    else                                                           -> autoTuneWorkers(cfg.ONNX, 0)
```

`openScriptResources` uses this for the auto-tuned default only. This is deterministic, needs no lock on a lazily-built pool, and is correct for the default configuration (batcher → 1).

*Alternatives considered.* (a) Read `Pool.Stats().NumWorkers` at first `emb.run` — rejected: it forces the pool to load (memory) purely to size another pool, and `NumWorkers` reports `1` for a batcher regardless of how many ORT sessions the batcher's session represents, making the number ambiguous. (b) Leave the bound as-is and only document `script_workers` — rejected: it leaves a measured +973 MB defect and a violated requirement.

### 2. An explicit `script_workers` is an override, not a hint

Only the auto-tuned default is clamped. An explicit value is opened verbatim, because scripted parallelism *is* a real axis for extraction workloads and the memory cost is the operator's call. The trade-off is documented in one table (sessions vs. memory vs. concurrency) so the choice is deliberate.

*Alternatives considered.* Clamping explicit values (today's behaviour) — rejected: it silently overrides configuration and contradicts the design intent recorded in the previous change.

### 3. Image caching is factored, not duplicated

Extract `Server.embedImages(entry, model, images) ([][]byte, error)` — the image analogue of `embedTexts` — owning the cache-read → preprocess+infer → cache-write sequence. `handleIMG` and the `emb.image.embed` host closure both call it.

This is the same shape as the text fix, keeps one cache-key definition, and makes the two paths provably equivalent. Batch semantics: the helper processes the requested images, reusing hits and inferring misses in one padded run, then writes results back.

*Alternatives considered.* Duplicating the cache logic in the script closure — rejected: two cache implementations drift, which is exactly how this defect appeared.

### 4. Image resources are resolved inside the closures

`runScripted` no longer calls `entry.ImageResources()`. The `emb.image.*` hosts are bound as closures over a `sync.Once`-guarded resolver, so the plan/sessions are created on first use — the same pattern already used for the script tokenizer and the embedding pool.

The image *plan* is needed to build the `ImageHost` (for `emb.image.info`). Since `Plan` is a value copied into the host, the resolver returns the plan lazily and `emb.image.info` reads it through the same guarded accessor.

### 5. Ownership and bounds are explicit

| resource | owner | released by | bound |
|---|---|---|---|
| named-tensor script sessions | `ModelEntry` (`scriptSessions`) | `Registry.Close` | `effectiveEmbeddingSessions` (default) or explicit `script_workers` |
| shared tokenizer | `ModelEntry` (`sharedTok`) | `Registry.Close`, exactly once | 1 per model |
| cached output tensors | `NamedRuntimeSession.outCache` | eviction, `Close` | `maxCachedOutputShapes` |
| script source / bytecode | `scriptCache` / `Compiler` | eviction, `EMB.SCRIPT FLUSH` | per-model caps |
| image sessions | `ModelEntry` (`ImageRes`) | `closeImageSessions` | `image_workers`/auto-tune (unchanged) |

Eviction destroys rather than unlinks; `Close` is idempotent; a partially-constructed pool closes what it already opened (already true, now covered by a test).

### 6. "Metal parity" is measured against the same graph run

Two baselines are used, because they answer different questions:

- **Sibling baseline** (`EMB` for the embedding class): answers "is the script as good as the command users already use?" — the existing 1.30×/1.15× budgets.
- **Bare-graph baseline** (the same script with its reduction removed): answers "how much does scripting itself cost?" — the new metal-parity budgets. This is the honest floor, because it holds the model, the session, the tokenizer and the graph constant and varies only the script machinery.

Measurement is done through `testing.Benchmark` in the budget tests, with a warm-up round before timing so lazy initialization is excluded.

## Risks / Trade-offs

- **[Default scripted parallelism drops from ~10 to 1 on batching-enabled models]** → this matches the embedding path's own concurrency (one session plus the batcher's async tokenization), which is the model the server already commits to; operators who need scripted parallelism set `script_workers` explicitly. The throughput-scaling budget verifies the override actually scales.
- **[Image caching adds memory]** → the image cache already exists for `EMB.IMG` and is bounded by the LRU byte budget; the script path now shares that budget rather than adding a new one.
- **[Deriving the bound from config could diverge from a later-loaded pool]** → the formula mirrors `ensurePool`'s worker computation, and a test asserts pool sessions and script sessions agree for both batcher and worker configurations.
- **[Lazy image resources change when the plan is validated]** → today the plan is validated on first scripted evaluation for image models; after the change it is validated on first `emb.image.*` use. Invalid image configuration still fails at model load (`validateImagePairing`), so this only moves the *session* creation.
- **[Leak tests can be flaky]** → follow the project's existing gated-fake/heap-probe style with no `time.Sleep` assertions, and use tolerances rather than exact equality for goroutine counts.

## Migration Plan

1. Land the session bound + explicit override; document the new semantics in `README.md` and the config reference.
2. Land the image cache sharing and lazy image resources (additive; no behaviour removed).
3. Land the leak tests; they must pass before archive.
4. Rollback is a revert per step; no persisted state is affected (script caches are content-addressed and unchanged).

## Open Questions

- Whether `EMB.STATS` should report scripted sessions as a ratio of the pool's session count (useful for operators tuning `script_workers`). Deferred: the absolute count is already reported; a ratio can be derived from `EMB.INFO`.
