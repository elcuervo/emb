# Proposal

## Why

Four defects in the Ruby client all share one root: a deferred or block call is not a snapshot of its arguments, and a batch window is not isolated per call. A caller who mutates a text or image String after deferring changes what the wire sends; a failure inside one client's serial batch raises on a *different* client's loader and drops its queued work; a mutated item also strands its telemetry entry, so the span never closes and the entry lives for the life of the thread; and coercing image bytes re-tags the caller's own String in place. Each was reproduced against the current gem; all four are invisible to the existing specs, which only mutate a *deferred image* payload and never put two clients in one scope.

## What Changes

- **Capture payloads at collection time.** A deferred text, a `client.multi` text, and a `multi` image payload are copied when the call is made (`.dup`, and `.b` for image bytes), so a later caller mutation cannot change the request and cannot change the item's hash identity.
- **Isolate a serial batch failure to its own slice.** When one client's share fails, only that slice's items resolve with the error; the remaining groups in the same batch window dispatch or fail on their own instead of inheriting it.
- **Make the telemetry deferral registry identity-keyed.** An entry is looked up by the item's identity, not by its (mutable) content hash, so a captured payload cannot strand it.
- **Stop mutating caller strings.** Image coercion returns a new binary String instead of re-tagging the caller's object.
- **Document the capture boundary** in the gem README: what a deferred call freezes (payloads, projection, format), and that identical deferred calls coalesce into one wire payload by design.

Not in scope: changing what the server sends, the `EMB.*` contract, eager single-call behavior (it sends immediately), or the parallel (`lazy: :batch`) failure path, which already isolates per share.

## Capabilities

### New Capabilities

_None._

### Modified Capabilities

- `emb-ruby-client`: adds requirements that deferred and multi-block calls snapshot their payloads, that a failing share cannot affect another client's call, and that coercion leaves the caller's object untouched.
- `ruby-client-telemetry`: changes how a deferral is keyed (identity, not content) so a resolution always finds and closes its entry.

## Impact

- `gems/emb/lib/emb/batch.rb` (item construction), `multi.rb` (`PairCollector`), `proxy.rb` (coercion, deferred capture), `batch_dispatch.rb` (serial failure isolation), `telemetry/deferred.rb` (keying).
- Specs: new examples for text mutation under `lazy:`, mutation inside a `multi` block, two clients in one scope with one failing, caller encoding after `image`, and deferral-registry drain.
- Docs: gem README capture and coalescing notes; CHANGELOG entry.
- No API change for correct callers; a caller who relied on mutating a String after deferring to change the request will now see the original bytes, which is the documented intent.
