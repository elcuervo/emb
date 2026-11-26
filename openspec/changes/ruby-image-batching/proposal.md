# Proposal

## Why

The Ruby client can batch several images supplied to one `proxy.image` call, but separate image calls remain eager and cannot join the request/job-scoped batches used by text embeddings. Image-heavy consumers must coordinate their own batches or pay extra round trips.

## What Changes

- Let image calls join the existing deferred scope under `lazy: :multi` and `lazy: :batch`, while preserving eager behavior by default.
- Coalesce deferred images by client and model; use bounded `EMB.IMG` chunks for same-model work, and follow each lazy mode's established serial/concurrent dispatch semantics for model groups.
- Add configurable `max_image_batch_bytes` limits to bound image count and total encoded bytes retained by a deferred scope.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `ruby-batch-loading` — define deferred image batching and its bounds, dispatch, result, and failure semantics.

## Impact

- `gems/emb` proxy, batch dispatch and scope lifecycle, focused specs, and client documentation. No server command changes or implicit URL fetching.
