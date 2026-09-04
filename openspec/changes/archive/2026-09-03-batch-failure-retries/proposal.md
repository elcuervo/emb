# Proposal: Batch failure retries with a typed server error

## Why

A failed `EMB.MULTI` batch currently raises the raw redis error once and resolves every
later use of the failed batch to a silent `[]` — no retry, no typed error for apps to
rescue. Callers must rescue `RedisClient::*` errors (an implementation detail) and cannot
distinguish "transient server trouble" from "the operation itself failed". This change
raises a dedicated `Emb::ServerError` with context once a batch fails, and lets operators
opt into bounded retries through the existing `reconnect_attempts` knob (default remains
`0`, fail-closed on the first attempt).

## What Changes

- Raise `Emb::ServerError` from the batch path instead of the raw redis error. It carries
  the model(s), text count, attempt count, and the underlying redis error as `cause`.
- Retries use the existing `reconnect_attempts` option as the single knob (no new option):
  with the default `0`, a failing batch fails closed after a single attempt; with a value
  `> 0`, a transiently failing `EMB.MULTI` is re-sent up to that many additional times
  before the batch fails closed. Retries are redis-client's own `ensure_connected` re-send
  loop (immediate, no backoff).
- Failure taxonomy, implemented by redis-client's existing classification:
  - **5xx-like (retried when configured)** — `RedisClient::ConnectionError`/`ProtocolError`:
    timeout, connection drop, server unreachable. Re-sent up to `reconnect_attempts` times.
  - **4xx-like (never retried)** — `RedisClient::CommandError`: the server parsed and
    rejected the command (unknown model, too many pairs, NOAUTH, ...). Raises
    `Emb::ServerError` after a single attempt under any configuration.
- **BREAKING**: the batch failure path raises `Emb::ServerError` instead of the raw redis
  error. Eager `Emb.multi` and `batch: false` paths are unchanged.
- Fail-closed tail unchanged after the final failure: pending items are removed from the
  scope, later resolutions return `[]` with no I/O, and later batches contain only newly
  deferred items.

## Capabilities

- **New Capabilities**: none.
- **Modified Capabilities**:
  - `ruby-batch-loading` — the "Batch failures fail closed" requirement is reworked into
    "Batch failures retry then fail closed": transient failures are re-sent up to
    `reconnect_attempts` additional times when configured; operation errors are never
    retried; the final failure raises `Emb::ServerError`.
  - `client-global-configuration` — an added requirement documents the opt-in retry
    semantics of `reconnect_attempts` (default `0` unchanged; `> 0` engages bounded batch
    retries that always terminate in `Emb::ServerError`).

## Impact

- **Code**: `gems/emb/lib/emb/errors.rb` (new `Emb::ServerError`), `configuration.rb`
  (comment only — the `reconnect_attempts` default of `0` is unchanged), `batch.rb` (typed
  raise with context instead of `raise e`). No new options, no new dependencies.
- **Docs**: `gems/emb/README.md` ("Why the timeout and reconnect defaults matter" and the
  "Fail-closed batches" section describe `Emb::ServerError`, the opt-in retry semantics,
  and a breaking-change callout).
- **Tests**: `emb_batch_spec.rb` (default single-attempt raise, configured retries
  counting `reconnect_attempts + 1` sends, exhaustion raises typed error with context,
  operation-error path, updated fail-closed specs), `configuration_spec.rb` / `emb_spec.rb`.
- **Backward compatibility**: apps rescuing `RedisClient::ReadTimeoutError`/`ConnectionError`
  around lazy batch forces must rescue `Emb::ServerError` instead (its `cause` exposes the
  redis error). Default retry behavior is unchanged (`0`). Eager APIs unaffected.