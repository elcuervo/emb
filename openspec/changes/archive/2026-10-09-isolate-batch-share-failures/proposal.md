# Proposal

## Why

Under `lazy: :batch`, a scope resolves into independent shares (one plain
`EMB <model> <text>...` per model, plus one per deferred script) that are
dispatched concurrently. Today a single share's terminal failure fails the whole
force and re-resolves the failed share's items to the `[]` default, so unrelated
healthy shares raise and failed shares silently read as empty. Real callers that
defer two independent models together (e.g. `siglip2` phrases and
`hyperclusters` clusters) lose one result to a sibling's failure and get `[]`
data corruption for the other.

## What Changes

- Under `lazy: :batch` (multi-share parallel scopes only), a terminal failure of
  one share no longer fails the force for the others. **BREAKING** (prerelease):
  the force no longer raises for a sibling's failure.
- Every item of a failed share raises that share's `Emb::ServerError` (same
  message/context as today, original redis error as `cause`) **on each use**,
  with no re-send and no I/O. **BREAKING** (prerelease): failed items no longer
  resolve to `[]`.
- The pending set is still emptied and failed commands are never re-sent.
- Non-redis errors (local bugs, e.g. `TypeError`) are unchanged: resolve the
  other shares, then clear the pending set and re-raise the original error.
- The serial path is unchanged and keeps fail-closed semantics: `:multi`, and a
  single-share scope sent on the forcing thread.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `ruby-batch-loading`: the "Batch mode parallel execution" requirement changes
  so a terminal share failure is isolated to its own items instead of failing
  the force and re-resolving the failed items to `[]`.

## Impact

- Code: `gems/emb/lib/emb/batch_dispatch.rb` (`resolve_outcomes`, new poison
  value and per-share error resolution) and `gems/emb/lib/emb/batch.rb`
  (`fail_batch!` becomes a non-raising `batch_error` builder).
- Tests: `gems/emb/spec/emb_batch_spec.rb` (two `:batch` examples updated). The
  serial fail-closed specs (`describe 'fail-closed batches with retries'`) stay
  unchanged.
- Docs: `gems/emb/README.md` and `gems/emb/CHANGELOG.md` note the behaviour
  change.
- No server, protocol, or configuration changes.
