# Design

## Context

See `proposal.md` and `specs/ruby-batch-loading/spec.md`. `Proxy#image` currently reads and normalizes all byte arguments, sends one eager `EMB.IMG`, and decodes keyed output maps. The existing deferred scope batches text and script items by client, with configurable `batch_size`, mode-specific dispatch, and request/job cleanup.

## Goals / Non-Goals

**Goals:** include image items in that existing scope, preserve eager return behavior, and bound both pending image count and retained bytes.

**Non-Goals:** change server image commands or preprocessing, fetch URLs, or make image output projections from separate calls interchangeable.

## Decisions

- Keep `lazy: false` unchanged. In deferred modes, normalize image inputs at call time so IO lifetime and later caller mutation cannot change the pending request; use bounded reads for IOs and duplicate mutable byte strings before retaining them.
- Add `max_image_batch_bytes` to global and per-client configuration, defaulting to 32 MiB (the server's default per-image byte cap). Reuse `batch_size` as the pending scope's image-count bound (and therefore each share's bound). Reject an item before registering it if it would exceed either limit for the pending scope; do not send or silently discard it.
- Store each image call as a batch item carrying client, model, bytes, format, output projection, and the call's result-shape metadata. Group only compatible items: one compatible group is one `EMB.IMG` share, since its image count is already capped by `batch_size`. `:multi` sends shares serially and `:batch` sends independent groups concurrently; cross-model `EMB.IMGMULTI` is the explicit `multi` block.
- Resolve loaders on the forcing thread, using the image reply decoder for keyed maps and preserving the existing single-image Hash versus multiple-image Array shape. Keep per-image null slots as `nil`; route terminal command failures through the established batch failure behavior.
- Track retained-byte accounting in the existing scope and release it with the existing request/job cleanup path, including exceptions. Avoid adding a second batch lifecycle or thread-local store.

## Risks / Trade-offs

- [Risk] Deferred image bytes increase per-request memory use → enforce count and byte limits before queue insertion, with a default byte cap below the server's default command cap.
- [Risk] Reading an IO at deferral time consumes it earlier than eager dispatch → document that deferred mode captures bytes immediately and reject over-limit input without queueing it.
- [Risk] Grouping by projection and format reduces coalescing opportunities → correctness takes precedence; incompatible replies must not share a command.
- [Risk] The separate image-multi API proposal may be implemented later → this design uses only the already-supported `EMB.IMG` command and has no dependency on that API.

## Migration Plan

No migration is required. Eager mode remains the default and keeps its current wire behavior. Deferred image work is opt-in through existing lazy modes; deploy the additive client release, monitor memory and batch outcomes, and roll back by disabling deferred mode or reverting the client release.
