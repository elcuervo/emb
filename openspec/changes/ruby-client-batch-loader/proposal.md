## Why

Embedding N texts via `Emb[:minilm][text]` currently costs N round trips to the server. The gem already ships an explicit `Emb.multi` batching API, but it requires manual collection and eager execution. We want automatic, lazy client-side batching: many `Emb` calls made in the same execution scope coalesce into a single `EMB.MULTI` command, without restructuring caller code.

## What Changes

- Add `batch-loader` as a dependency of the `emb` gem.
- Add an explicit lazy batching API: `Emb.batch[:model]["text"]` (and `client.batch[...]` for instances) returns a lazy embedding that materializes on first use. Loaders created in the same thread share one batch key — a single constant batch block keyed `:emb` — and are sent as a single `EMB.MULTI` command per client, regardless of model: mixed-model batches are one round trip.
- Add a `batch` configuration option (default `false`): when `true`, the existing `Emb[:model]["text"]` / `client[:model]["text"]` proxy calls return the lazy batched embedding instead of hitting the network immediately.
- Behavior parity with the eager API: single text returns `Array<Float>`, multiple texts return `Array<Array<Float>>`, failures surface as `nil` per pair (MGET semantics).
- Cache embeddings per execution scope (thread) with `cache: true`, so repeated uses of the same lazy value are free.
- `Emb.multi` remains the explicit, eager batching API — unchanged and complementary.

## Capabilities

### New Capabilities

- `ruby-batch-loading`: lazy client-side batching of embedding calls via batch-loader, coalescing per-thread calls into `EMB.MULTI`, plus the `Emb.batch` API surface.

### Modified Capabilities

- `emb-ruby-client`: add the `batch` configuration option and define how the proxy-based API behaves in batch mode (lazy vs eager).

## Impact

- **Gem**: `gems/emb` — new dependency on `batch-loader` (zero-dependency gem), batch implementation in `lib/emb/batch.rb` (a single constant batch block keyed `:emb` plus `Emb::BatchProxy`), `Client#batch` config option, `Proxy#[]` behavior switch.
- **Server**: none — reuses existing `EMB.MULTI`.
- **Docs**: `gems/emb/README.md` gains batch usage and the batch-loader contract (create loaders, then consume; per-thread scope).
- **Tests**: `gems/emb/spec/` gains batch tests using a fake/stubbed client.