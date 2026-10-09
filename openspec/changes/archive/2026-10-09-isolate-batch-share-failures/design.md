# Design

## Context

See `proposal.md` - Why. The relevant current state lives in
`gems/emb/lib/emb/batch_dispatch.rb` and `gems/emb/lib/emb/batch.rb`:

- `BATCH_BLOCK` chooses `dispatch_parallel` only when `parallel_batch?` is true
  and the scope resolves into more than one share; otherwise it uses
  `dispatch_serial`. A lone parallel share therefore takes the serial path.
- `dispatch_parallel` runs one worker per share, captures each result as
  `[:ok, slice, reply]` or `[:error, slice, error]`, then calls
  `resolve_outcomes` on the forcing thread (batch-loader resolution must happen
  there).
- `resolve_outcomes` currently resolves the `:ok` shares and then, if any
  outcome carries a redis error, calls `fail_batch!`, which clears the pending
  set and raises `Emb::ServerError` on the forcing thread. batch-loader then
  never runs its `default_value` fallback for the failed items, so their later
  `loaded_value` is `@default_value.dup` (`[]` for embeds) from
  `BatchLoader::ExecutorProxy#loaded_value`.
- `dispatch_serial` preserves the fail-closed behaviour for `:multi` and for
  single-share `:batch` scopes and is not changing.

## Goals / Non-Goals

**Goals:**

- A share's terminal redis failure is contained to that share's items on the
  parallel path: healthy shares resolve, the force does not raise for a sibling.
- Every item of a failed share raises that share's `Emb::ServerError` (same
  message/context, original redis error as `cause`) on each use, with no I/O.
- Failed items never resolve to `[]`.
- Non-redis errors keep today's semantics: resolve the other shares, clear the
  pending set, re-raise the original error.
- Keep the serial path exactly as-is.

**Non-Goals:**

- Changing the `:multi` path, single-share `:batch` scopes, MGET per-pair `nil`
  semantics, retry budgeting, or the `Emb::ServerError` message format.
- Adding a public API to inspect a failed item; the only observable change is
  that using it raises.

## Decisions

### Isolate per share in `resolve_outcomes`, not in the workers

Workers only capture `[:error, slice, error]`; resolution must stay on the
forcing thread because batch-loader's executor is per-thread. So the decision
point is `resolve_outcomes`, which now walks every outcome:

- `:ok` -> `resolve_slice`. If that itself raises (e.g. `ShortReplyError` from a
  short reply, which subclasses `RedisClient::ProtocolError`), treat that share
  as failed rather than letting it tear down the batch.
- redis error -> resolve each item of the slice to a poison value.
- non-redis error -> remember the first one and continue resolving the rest.

After the loop, only a remembered non-redis error triggers
`clear_batch_pending!` + `raise`. The redis path does not raise, so batch-loader
finishes `__ensure_batched`, prunes the pending items itself, and stores each
poison value as that item's loaded value.

Alternative considered: keep raising on the force but only when the forced item
belongs to the failed share. This still leaves the other failed items resolving
to `[]` (the silent-empty symptom), so it does not meet the requirement.

### Poison value instead of raising from the block

batch-loader has no per-item error channel: the batch block either returns or
raises for the whole scope. The loaded value is what a caller later dereferences,
so the error must live in the value. A `BasicObject` (`FailedShare`) whose
`method_missing` raises satisfies this:

- `value.methods` inside batch-loader's `__replace_with!` on first use raises.
- If `replace_methods`/`cache` were disabled, the loader's own `method_missing`
  still ends in `public_send` on the poison, which raises via `method_missing`.
- `Array(loader)` and `respond_to?` raise too, so a failed item cannot be
  silently coerced to `[]`.
- `@synced` short-circuits later calls before any executor work, and the item is
  already loaded, so re-use never re-sends.

No `respond_to_missing?`: `BasicObject` has no `respond_to?`, so it would be
dead code.

Alternative considered: resolve failed items to a lambda or a custom error
object the caller must check. That leaks type checks into callers and can still
be mistaken for a value; the poison is fail-loud.

### Non-raising `batch_error` builder, `fail_batch!` removed

`fail_batch!` combined two jobs: build the `ServerError` and clear+raise it. The
parallel path needs to build the error once per failed share and hand it to the
poison without clearing or raising on the forcing thread. So it is split:

- `batch_error(error, slice:, budget:)` computes `attempts`, models and text
  count, builds the message, and attaches `error` as `cause`, returning the
  exception. Because the parallel path is not inside a rescue of the original
  error, the cause is attached with the raise-inside-rescue idiom (raise the
  original, build the wrapper inside that rescue, then rescue the wrapper to
  return it) - the only way to set `cause` from a non-rescue context.
- `dispatch_serial` now does `clear_batch_pending!; raise batch_error(...)`,
  preserving today's serial behaviour byte-for-byte.
- `collect_outcomes` is removed.

## Risks / Trade-offs

- **The poison raises on any probe** (`nil?`, `empty?`, `respond_to?`) -> this is
  the point: failed items must not read as `[]`. Acceptable because the
  alternative is silent data loss; document the change in CHANGELOG/README.
- **Reliance on batch-loader's replace flow** -> the poison raises on first use
  via `__replace_with!`, and also via the loader's `method_missing` if replace
  is off, so behaviour does not hinge on one gem internal. The updated spec
  exercises `.first` and repeated use.
- **One error object shared by a share's items** -> raising the same exception
  instance repeatedly is safe and keeps the message/context identical for all
  items of the share.
- **Behaviour change on a prerelease** (`0.4.3.pre4`) -> no migration needed;
  callers previously relying on `[]` for a failed batch must rescue
  `Emb::ServerError` instead. Note it as a breaking change.

## Migration Plan

Ship in the unreleased prerelease; no data or API migration. Rollback is
reverting the two library files and the spec examples.
