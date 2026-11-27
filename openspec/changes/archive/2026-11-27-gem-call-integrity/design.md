# Design

See `proposal.md` for motivation; see the two delta specs for required behavior.

## Context

- `build_batch_loader` builds `[client, model, text, format]` with the caller's String (`batch.rb:75`); `MultiProxy::PairCollector` does the same for text and stores `Proxy.coerce_image_bytes(payload)` for images (`multi.rb:78-84`). Only `deferred_image` copies (`proxy.rb:96`).
- That Array is simultaneously batch-loader's dedupe key, `ImageBatch`'s pending-usage key, and `Telemetry::Deferred`'s Hash key. A mutable element makes all three content-dependent.
- `dispatch_serial` rescues a failed share and calls `clear_batch_pending!`, which deletes the whole `[BATCH_BLOCK.source_location, BATCH_KEY]` set (`batch.rb:135-147`) — every client's queued items in that thread. `BATCH_BLOCK` iterates `items.group_by(&:first)`, so one client's failure aborts the rest.
- The parallel path already isolates per share: `FailedShare` poisons only that slice's items (`resolve_outcomes`).
- `coerce_image_bytes` re-tags the caller's String (`proxy.rb:71-76`); the spec asserts the wire encoding, not the caller's object.
- Repro evidence (current tree): deferred text sends `hello MUTATED`; multi image sends `abcZZZ`; client B's loader raises client A's `ServerError`; the deferral registry retains 1 entry after a mutated-text resolution; a UTF-8 String becomes `ASCII-8BIT` in place.

## Goals / Non-Goals

**Goals:**
- A deferred or block call's wire content is fixed at call time.
- A failure is attributable to the calls it actually carried.
- The deferral registry drains regardless of payload aliasing.
- Coercion is a pure function of its input.

**Non-Goals:**
- Changing server behavior, the `commands.json` contract, or eager single-call semantics.
- Reworking batch-loader or replacing the per-thread scope with an explicit one.
- Changing the dedupe semantics themselves (identical deferred calls still share one payload) — only making them independent of caller mutation.

## Decisions

### Copy at collection, in the two constructors that accept a caller object

`build_batch_loader` stores `text.dup` (leaving a frozen literal's fast path via `-text.to_s`? no — plain `dup`), and `PairCollector` stores `text.dup` and `payload.b`. A single copy per call is the cost; it is O(payload) and dominated by the network write it precedes.

- *Alternative:* freeze the caller's object. Rejected: mutates their state worse than an encoding change, and raises where they had a working program.
- *Alternative:* copy at dispatch. Rejected: the caller has already been able to mutate, which is the bug.

### Identity-keyed deferral registry

`Telemetry::Deferred` keeps `deferrals` keyed by the item Array today. Store the entry on a per-scope store keyed by `item.object_id`-backed wrapper or, simpler, keep a `HashMap<object_id, entry>`; `record`, `dispatched`, and `resolve` use `item.object_id`. Items are alive from record to resolve, so `object_id` cannot be reused in that window.

- *Alternative:* compare by identity in a linear scan. Rejected: O(n) per resolve on every deferred call.
- *Alternative:* stop keying by the item at all and pass the entry through the loader. Rejected: touches every dispatch path; the registry already isolates this concern.
- Note: copying payloads (`text.dup`) makes the current content-keyed Hash stable as well, so this decision is defense in depth against aliasing, not the only fix.

### Isolate the serial path the way the parallel path already does

`dispatch_serial` gains the same per-slice isolation: on a share's failure, resolve that slice's items with a `FailedShare` (or the batch error) and continue with the remaining slices, instead of `clear_batch_pending!` plus re-raise. The pending set is pruned per slice (`items_by_block[key] -= slice_to_items(slice)`), leaving other clients' items queued. A non-Redis error (local bug) keeps today's behavior: clear and re-raise.

- *Alternative:* keep clearing the scope and only re-attribute the error. Rejected: B's work would still be silently dropped.
- *Risk:* `ShortReplyError` accounting (`batch_error` attempts, retry classes) lives in the same rescue; the new path must build the same `ServerError` per failed slice.

### Coercion returns a new binary String

`coerce_image_bytes` returns `bytes.b` for strings (a fresh String with `ASCII-8BIT` encoding) and `io.read`'s result for IOs; the `dup if frozen?` branch disappears because the value is always new.

- *Alternative:* keep `force_encoding` and document the mutation. Rejected: a String's encoding is observable state; a client library should not change its caller's object.

## Risks / Trade-offs

- [One extra String copy per text under `lazy:`] → Measured cost is per call, not per byte sent (text is small); images already copied on the deferred path.
- [Per-slice isolation changes fail-closed behavior] → The existing batch-error specs (`emb_batch_spec`) encode the current expectations; they must be re-run and updated only where they asserted scope-wide clearing.
- [`object_id` keying assumes the item outlives the entry] → True by construction (entry is deleted at resolve, before the loader drops the item); add the drain assertion so a regression is visible.
- [A user who mutated a String to alter the deferred request loses that ability] → That was never documented and contradicts "captured at call time"; the README will state the snapshot boundary.

## Migration Plan

Behavior-compatible for correct callers: no API change, no dependency change, no wire change. Release as a patch on the gem; CHANGELOG notes the snapshot guarantee and the isolation fix. Rollback is reverting the commit; no data or state to migrate.

## Open Questions

None that change the approach. Whether the *eager* multi block should also snapshot image bytes is settled by this change (yes — same constructor), not deferred.
