## Context

See `proposal.md` — Why. The server stores its runtime caps (`max_texts`, `max_pairs`, `max_images`, `max_image_bytes`, `max_image_pixels`, `max_command_bytes`) in plain `int`/`int64` fields. Those fields were boot-only until `CONFIG SET` made them writable, and the request paths read them without synchronization. The cache stores caller-provided `[]byte` values verbatim; embedding rows are sub-slices of one batch-wide buffer. The script reply cache keys by model + script SHA1 + ARGV digest + one text, but interprets a script's return value differently for one text (the whole value) than for several (one element per text). The Ruby client's `ready?` is built on `ready`, which already rescues command errors into a string.

## Goals / Non-Goals

**Goals:**
- Zero `-race` reports for concurrent `CONFIG SET` + inference.
- `CONFIG GET` always reports live values.
- Cache values owned by the cache; accounting proportional to retained bytes.
- Script replies never replayed across call shapes.
- Ruby `ready?` is a truthful predicate; `protocol` cannot select an unsupported mode.

**Non-Goals:**
- Implementing RESP3 decoding in the Ruby gem (only rejecting unsupported `protocol` values).
- Changing the `EMB`/`EMB.MULTI` wire shapes or the cache's persistence format.
- Making `CONFIG GET`/`SET` parameter names case-insensitive (a Redis nuance tracked separately).
- Rewriting the pool's shared-buffer row views (only the cache boundary is fixed).

## Decisions

1. **Atomic caps, not a lock.** Represent each of the six caps as `atomic.Int64` and expose small `maxX()` accessors. Reads sit on the hot request path; an `RWMutex` would add contention for no benefit, and `atomic.Int64` is the same pattern already used for `password`/counters. Alternatives: a mutex-guarded struct (hot-path contention, larger diff) or leaving the race as "benign on 64-bit" (rejected — the race detector fails and the memory model gives no guarantee).

2. **`cacheConfig` becomes an `atomic.Value`.** `CONFIG SET cache` stores the operator's input string (what Redis echoes), and the getter loads it. Alternative: recompute the display string from `cache.Stats().MaxBytes` — rejected because it would round human units (`128mb` → `128000000`) and the existing contract echoes the configured form.

3. **Arity in the script cache key.** Fold the text count into the existing SHA-256 argument digest (model + script SHA1 + count + ARGV + text). This keeps the "large text is digested, not inlined" property and separates the single/multi namespaces. Alternative: cache a canonical per-text value in both modes — rejected because a single-text script's contract genuinely returns the whole value, and forcing a wrapper would change the reply shape.

4. **Copy on `Set`, once.** `Cache.Set` copies the value bytes before storing. The copy cost is small (one embedding row, ~1.5 KB) relative to inference, and it fixes every caller (text, image, future) at one boundary instead of auditing each sender of sub-slices. Alternative: allocate each row independently in the pooling path — rejected because it moves the copy into the hot fan-out and still leaves any other aliasing caller broken.

5. **Ruby `protocol` validation at both entry points.** `Configuration#protocol=` validates so `Emb.configure` fails fast, and `Client#initialize` validates the merged options so per-client `Emb.new(protocol: 3)` also fails. Alternative: only validate in `Configuration` — rejected because `Emb.new(protocol: 3)` bypasses it.

6. **`ready?` derived from `ready`.** `ready?` returns `ready == "OK"`, rescuing connection errors to `false`, so `ready` remains the reason-bearing accessor and both share one command path.

## Risks / Trade-offs

- [Script cache keys change, so persisted script replies miss once after upgrade] → Acceptable: it is a one-time miss, the snapshot still loads, and the alternative is serving wrong values.
- [Rejecting `protocol: 3` removes an option someone may have set] → It never produced correct introspection results; raising is strictly safer than corrupting, and the specs and README state RESP2-only.
- [Copy-on-`Set` adds allocation on each cache write] → Bounded by one small slice per embedding; measured against inference cost it is noise.
- [`atomic.Int64` with `CONFIG SET` still does not make a multi-cap request atomic] → Not required: each command reads each cap once; a set taking effect at the next command is the documented semantics.

## Migration Plan

Server: no config or data migration. Cache files stay readable; new script keys simply miss on first use. Ruby gem: release as a normal minor bump; a caller setting `protocol: 3` now gets a loud `ArgumentError` instead of silent wrong data — call it out in the changelog. Rollback is a reinstall; no persistent state depends on the fix.

## Open Questions

None.
