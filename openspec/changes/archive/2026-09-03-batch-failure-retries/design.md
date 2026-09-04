# Design: Batch failure retries with a typed server error

## Context

The lazy batch path (see proposal.md — Why) currently raises the raw redis error once and
fails closed. The fail-closed mechanics — `clear_batch_pending!` keyed on
`[BATCH_BLOCK.source_location, BATCH_KEY]`, `default_value: []`, memos in
`loaded_values_by_block`, thread-local `BatchLoader::Executor` — are proven and stay as-is;
this design only changes *what error type* the batch path raises and *what the
`reconnect_attempts` knob does* for batches.

## Goals / Non-Goals

- **Goals**: one stable `Emb::ServerError` type with cause + context; bounded retries for
  transient failures, driven by the existing `reconnect_attempts` knob (no new option);
  default behavior unchanged (`reconnect_attempts: 0`, fail-closed on the first attempt);
  fail-closed behavior unchanged once the error surfaces.
- **Non-Goals**: a new retry option (`batch_retries`) — rejected, one knob only; retrying
  eager paths (`Emb.multi`, `batch: false` proxies) — unaffected; backoff scheduling —
  redis-client retries with 0-delay by default; per-pair null handling (MGET semantics
  already cover it); changing the `reconnect_attempts` default.

## Decisions

### D1 — One knob: reuse `reconnect_attempts` (no `batch_retries`)

A separate `batch_retries` option was drafted and dropped: retries are already implemented
by redis-client's `ensure_connected` loop (`redis_client.rb:769-809`) — re-sending inside
`call` on failure, up to `reconnect_attempts` times, with 0-delay between tries by default.
A parallel gem-level retry loop would either compound with it or fight it. The knob is the
existing option; the batch layer only observes the outcome.

### D2 — Default stays `0`; retry is opt-in

The proposal's draft changed the default to `2`; the decision is to keep `0` (the
secure-defaults stance from `secure-client-batch-defaults`): by default a batch fails
closed after one attempt, and operators opt into bounded retries by setting
`reconnect_attempts > 0`. The user-visible contract ("retry, then raise a typed error") is
preserved for opt-in configurations, and every exhausted batch — default or configured —
terminates in `Emb::ServerError`, never an endless silent re-send.

### D3 — Taxonomy is redis-client's own (the 5xx/4xx split)

- **5xx-like (retried when configured)**: `RedisClient::ConnectionError` / `ProtocolError` —
  timeout, connection drop, server unreachable. `ensure_connected` rescues exactly these
  (`redis_client.rb:793`) and re-sends; after `reconnect_attempts` retries it raises the
  last error. `ReadTimeoutError < TimeoutError < ConnectionError`, so read/write timeouts
  are in this bucket.
- **4xx-like (never retried)**: `RedisClient::CommandError` — the server parsed and rejected
  the command (unknown model, NOAUTH, too many pairs, …). Not a `ConnectionError`, so it
  propagates immediately regardless of `reconnect_attempts`.

No new taxonomy is defined; the spec phrases this in terms of behavior, the implementation
inherits redis-client's classes. `Emb::ServerError` carries them as `cause` either way.

### D4 — batch.rb: wrap the final failure in `Emb::ServerError`

The block's rescue changes from `raise e` to a `fail_batch!` helper:

```ruby
begin
  results = Array(client.send_command('EMB.MULTI', *pairs))
rescue StandardError => e
  fail_batch!(e, slice: slice, budget: retry_budget(client))
end
```

`fail_batch!` runs `clear_batch_pending!` (fail-closed) then raises `Emb::ServerError`
with the cause attached automatically by Ruby. `attempts` is nominal: redis-client made
`budget + 1` sends by the time it raises a transient error (`ConnectionError` or
`ProtocolError` — both are re-sent by `ensure_connected`); anything else is 1. `budget`
resolves like `batch_size` (per-client `reconnect_attempts` accessor or
`Emb.configuration`), normalized to an Integer (redis-client also accepts a delay Array).

### D5 — `Emb::ServerError` shape

`class ServerError < StandardError; attr_reader :attempts` — positional message, keyword
`attempts:` (default 1), so plain `raise Emb::ServerError, 'msg'` works too.
Message (examples): `EMB.MULTI failed after 3 attempt(s) (models: minilm, 512 text(s)) RedisClient::ReadTimeoutError: read timed out`.
No `Emb::Error` base class yet — one error type is all this change needs; a base is
trivially additive later.

### D6 — Fail-closed tail unchanged

After the raise: pending cleared, later resolutions `[]` with no I/O, later batches contain
only newly deferred items. Proven batch-loader mechanics; the existing specs keep their
scenarios (reworded to expect `Emb::ServerError`).

## Risks / Trade-offs

- **Duplicate inference when retries are configured** (a slow server re-embeds the chunk up
  to `reconnect_attempts` extra times). → Opt-in only; bounded by `reconnect_attempts`;
  always terminating in a visible `Emb::ServerError`; documented.
- **Worst-case latency stacks when configured**: `reconnect_attempts + 1` × 10s timeout
  before the raise. → Documented; default (`0`) unaffected.
- **Breaking change**: rescue sites must switch from `RedisClient::*` to `Emb::ServerError`
  around lazy batch forces. → `cause` preserves the original error; README breaking-change
  callout; eager paths untouched.

## Migration Plan

Gem release notes call out the breaking change: lazy-batch failures now raise
`Emb::ServerError` (rescue `Emb::ServerError`, read `cause`). Default retry behavior is
unchanged (`reconnect_attempts: 0`); operators who want bounded transient retries set
`reconnect_attempts` to a value greater than zero. No data or server-side changes;
rollback = reverting the gem change.

## Open Questions

None — the decisions resolve all spec-visible behavior; the opt-in-only retry stance and
the reuse-`reconnect_attempts` choice are explicit in the proposal and specs.