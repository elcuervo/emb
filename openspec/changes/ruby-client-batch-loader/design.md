# Design: Ruby Client Batch Loading

## Context

`emb`'s Ruby gem (`gems/emb`) exposes `Emb[:model]["text"]` which issues one `EMB` command per
call. The server already supports `EMB.MULTI` (alternating `model text` pairs returning an
ordered array, MGET-style partial failures as null per pair), and the gem has an explicit
`Emb.multi` API requiring manual pair collection and eager execution. There is no automatic
batching: N embedding calls in application code cost N round trips, even in a single
request/thread with a warm connection.

This change adds lazy, automatic client-side batching using the `batch-loader` gem (a
zero-dependency lazy batching mechanism): embeddings requested within the same execution
scope (thread) coalesce into one `EMB.MULTI` round trip.

The `batch-loader` semantics were verified against the gem source and an empirical probe:

- Items register at **creation** time; the batch block fires once on **first use** (any
  method dispatch on the lazy object — `method_missing` triggers `__sync`).
- Batching scope is `Thread.current`; the batch key is `[block.source_location, key]`.
- `cache: true` (default) memoizes loaded values per scope; repeated use is free.
- Identical items deduplicate (Set-backed registry); a batch block that receives a failed
  pair floods remaining unloaded items with `default_value`.
- The **batch block must be a constant** (single `source_location`) or carry a fixed `key:`,
  otherwise loaders created at different call sites land in different buckets and batching
  silently never happens.

## Goals / Non-Goals

**Goals:**

- One `EMB.MULTI` round trip for N embed calls within the same thread/scope.
- Explicit `Emb.batch[:model]["text"]` (and `client.batch[...]`) lazy API, always available.
- `batch: true` configuration option making the standard proxy API lazy; default `false`
  keeps today's behavior byte-for-byte identical.
- Value shape parity with the eager API (single → `Array<Float>`, multiple → `Array<Array<Float>>`).
- Cached per-scope values (`cache: true`).
- `Emb.multi` remains the explicit, eager, transactional escape hatch.

**Non-Goals:**

- Cross-thread batching (the server's own per-model smart batching covers concurrent
  inference coalescing; the scope of this change is client round trips).
- Server-side changes of any kind.
- A "flush everything" API — the batch contract is that values materialize on use.
- Changing `Emb.multi`'s behavior or API.

## Decisions

### D1: One global batch block keyed to the gem, not per-model

A single constant batch block (`key: :emb`, or the constant's own `source_location`) serves
all models. Items are `[model, text_or_texts]` pairs; the block flattens them into
`EMB.MULTI` args for one server round trip.

- **Alternative considered:** per-model batch keys. Rejected — a thread mixing `minilm` and
  `bge` calls would produce two MULTI commands instead of one. The server parallelizes pairs
  internally (goroutine per pair), so a mixed single call is strictly better.

### D2: Proxy-style chain for `Emb.batch`

`Emb.batch` (module-level, backed by the default client) and `client.batch` return a
`BatchProxy` mirroring `Proxy`'s memoized `[]` chain. `Emb.batch[:minilm]` returns a
per-model memoized lazy proxy; `["hello"]` returns the lazy value for that text. This keeps
call sites visually identical to the eager API.

### D3: Multi-text calls are one item, expanded in the block

`Emb.batch[:minilm]["a", "b"]` registers a single item `[:minilm, ["a", "b"]]`. The block
expands it into two `EMB.MULTI` pairs and regroups results; the materialized value is
`vectors.first` when one vector, the array otherwise — exactly the eager `Proxy#[]` shape.
This keeps "behavior same as original API" for multi-text without fragmenting the batch.

### D4: `batch: true` switches `Proxy#[]` to the lazy path

`Client` gains a `batch:` option (default `false`). `Proxy#[]` consults it: eager (current
code) or lazy (register item, return `BatchLoader.for(...).batch(...)`). The lazy path always
uses `EMB.MULTI` as its transport, even for a single text — the block is the same.

### D5: Cache per scope with `cache: true`

Default batch-loader caching is kept: the first use of a value in a thread is a MULTI
round trip; subsequent uses and identical pairs are served from the scope cache. This matches
the "same behavior as original API" goal (the same text produces the same embedding) and
makes repeated in-thread access free.

- **Trade-off accepted:** the per-thread cache grows with distinct texts in long-lived
  threads. Embedding vectors are small (KB-scale) and the server also caches internally;
  unbounded growth in a worker process is an accepted risk for v1 (see R2).

### D6: Failed pairs materialize as `nil`

The block calls `loader.call(item, nil)` when the server returns null for a pair (unknown
model or inference error). Using the value then behaves exactly like operating on `nil` —
checkable via `.nil?`, raising `NoMethodError` otherwise — the identical behavior of
today's eager `Emb.multi` on partial failure.

- **Alternative considered:** raise from the block on any nil. Rejected — one failed pair
  would discard all healthy siblings' results, violating the MGET bargain, and errors inside
  the block can't be stored per-item by batch-loader, risking re-fire storms.

### D7: `Emb.multi` and the eager path stay untouched

`Emb.multi`/`MultiProxy` remain the explicit eager API. In `batch: false` mode nothing
changes at all. `Emb.multi` is the escape hatch when callers want deterministic,
non-lazy batching (e.g., transactional updates, cross-thread coordination).

## Risks / Trade-offs

- **[Silent drop / delayed work]** With `batch: true` or `Emb.batch`, a loader created but
  never used only embeds if a sibling batch fires in the same thread; otherwise the inference
  never happens (and normalization of loading time shifts from call time to first use).
  → Mitigation: document the contract prominently in the gem README ("create loaders, then
  consume"); keep `batch` defaulting to `false`; `Emb.multi` remains for deterministic code.
- **[N+1 if interleaved]** Creating and using a loader in the same iteration
  (`texts.each { |t| process(Emb.batch[:m][t]) }`) yields one MULTI per item — no better than
  eager, no worse. → Mitigation: README examples show the create-then-consume pattern.
- **[Batch key fragility]** If the batch block is ever defined inline per call site instead of
  as a fixed constant/key, `source_location` bucketing breaks batching silently.
  → Mitigation: implementation uses a single constant block + fixed `key:`; a spec test
  asserts two loaders created from different call sites still coalesce.
- **[Unbounded per-thread cache]** Long-lived threads accumulate distinct-text embeddings.
  → Mitigation: accepted for v1; repeal point is a per-scope LRU or explicit clear; `nil`
  results are also memoized (server doesn't cache failures), so repeated failed pairs don't
  re-send.
- **[Per-thread only]** Batching does not span threads; a multithreaded app still issues one
  MULTI per thread. → Mitigation: server-side smart batching coalesces concurrent inference
  server-side; this change targets round-trip reduction within a scope.

## Migration Plan

- Add `batch-loader` to `emb.gemspec` dependencies (zero-dependency gem; no peer
  constraints; publish with next gem release).
- `batch` option defaults to `false` — existing applications are unaffected without opt-in.
- Rollback: remove the `batch` option / revert proxy routing; explicitly lazy call sites
  (`Emb.batch`) are isolated and removable independently.

## Open Questions

- Should `Emb.batch_flush!` (force current scope's pending loaders to fire) be added in a
  later iteration? Current stance: no — the contract is "materialize on use", and a flush
  would require reaching into `BatchLoader::Executor` internals.
- Does the server's own smart batching make client-side batching redundant for high-concurrency
  workloads? This change is motivated by round-trip reduction; server-side batching addresses
  inference coalescing — both compose, but a benchmark comparing eager+smart-batch vs
  lazy-batch would quantify the win.