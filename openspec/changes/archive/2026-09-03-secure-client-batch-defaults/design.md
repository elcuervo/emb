# Design: Secure client batch defaults (timeouts, reconnect, fail-closed batches)

## Context

See `proposal.md` Why. The mechanism is reproduced and **validated** by `bench/repro/client-timeout/repro.rb` (19 assertions, exit 0), with the key facts pinned to library source:

- redis-client `DEFAULT_TIMEOUT = 1.0` (`config.rb`); the gem passes `read_timeout: nil`, which forwards nothing, so redis-client applies 1.0s.
- `ReadTimeoutError < TimeoutError < ConnectionError` (`redis_client.rb:167-173`) → `ensure_connected` treats it as a connection failure and **re-sends the whole command** up to `reconnect_attempts` times (measured: 4 `connection.call` attempts with the gem's default 3).
- batch-loader `__ensure_batched` calls `delete(items:)` only after the batch block returns; a raise leaves **every item** in `Executor` (a thread-local `Set`), so the next force re-runs the batch from scratch.
- `Respond_to?`/`method_missing` overwrite the loader with the loaded value (`replace_methods`), so memoizing an error object would surface `NoMethodError` on the error instead of the error itself — fail-closed must NOT memoize the error.

## Goals / Non-Goals

**Goals**
- Defaults that cannot silently duplicate server work (no 4× resend, no retained re-run loop).
- Fail-closed batches: errors surface once; retries are inert; pending sets are bounded.
- Keep the reproduction as a committed regression script.

**Non-Goals**
- Server-side changes (caps, single-flight) — separate concerns; the validated leak is client-side.
- Changing redis-client itself (no fork/override).
- Retry policies above the gem (app/job middleware stays the caller's choice).

## Decisions

### D1 — Configuration defaults: `read_timeout`/`write_timeout` 10s, `reconnect_attempts` 0

- **Why 10s**: covers the shipped 512-pair batch during worst-case inference (shared CPU, cold cache, GC pauses) with headroom, while still surfacing a genuinely dead server in bounded time. It is a floor to document, not a promise of latency.
- **Why 0 reconnects**: `EMB.MULTI` is not idempotent — every automatic re-send re-runs server inference. With fail-closed batches (D2), the app can retry safely at the job/request layer; the gem must not amplify beneath it.
- Alternatives: derive timeout from `batch_size` — rejected (breaks when `batch_size` changes at runtime and modeling per-model latency is guesswork); keep 3 reconnects — rejected (validated 4× work amplification).

### D2 — Fail-closed `BATCH_BLOCK`: clear pending + re-raise once

On a command failure inside `BATCH_BLOCK`: re-raise the original error (surfaces once to the forcing caller) and **remove every deferred item from the executor's pending set** so nothing can be re-sent:

```ruby
rescue => e
  clear_batch_pending!   # Executor.current.items_by_block.delete([BATCH_BLOCK.source_location, BATCH_KEY])
  raise e
end
```

- `items_by_block` is keyed `[block.source_location, key]`; the block's own source_location + `Emb::BATCH_KEY` is the only stable public handle (batch-loader's `delete` needs the internal proxy, unreachable from the block). Guard against a nil current executor (no scope).
- Post-failure re-resolution: pending set empty → batch block runs with no items → no I/O → `loaded_value` returns the default (`nil`). No error memoization (see Context) — documented as "re-resolution returns nil".
- Partial-chunk failures: already-loaded chunk items keep their real values (memoized via `loader.call` before the raise); only the unsent remainder is cleared.
- This is exactly the behavior Phase D of the repro validates (error once, retry inert, pending stays 0, subsequent batch carries only new items).

### D3 — Committed repro as regression evidence

`bench/repro/client-timeout/` (mock RESP2 server, driver, README) stays in the repo: run as `cd gems/emb && bundle exec ruby ../bench/repro/client-timeout/repro.rb`. After the defaults land, Phases A/B change meaning (the loop no longer triggers by default); the README documents running with `MOCK_DELAY=15` to still exercise the raw mechanism or treating it as a guard that the loop stays dead (its C/D phases double as the fix's acceptance tests).

## Risks / Trade-offs

- [Re-resolution of failed items yields `nil`] → Callers that previously relied on re-forcing to eventually succeed now see `nil`; they SHALL treat it as the documented fail-closed signal and do app-level retry (which is safe). Spec scenario pins this.
- [10s timeout delays error visibility for a dead server] → Bounded and far better than the previous alternatives (1s failure under load; or no enforcement at all on some code paths). Tunable via `Emb.configure`.
- [`reconnect_attempts: 0` makes transient blips fail the batch immediately] → Correct posture: fail once, let the app retry; no hidden duplicate work.
- [Clearing the pending set deviates from batch-loader's default retention] → Intentional and scoped to failure; success paths and per-pair server-null semantics (MGET-style) are unchanged.

## Migration Plan

Pure client behavior change, no wire impact:
1. Land defaults + fail-closed + specs + README + repro (commit bundle with the change artifacts).
2. Release per the `gems-release-lifecycle` flow; changelog highlights the default changes (operators relying on auto-resend or the 1s timeout must set `Emb.configure` explicitly).
3. No server rollback concerns.

## Open Questions

None blocking. (Re-resolution returning `nil` vs re-raising the stored error is resolved toward `nil` to avoid the `method_missing` replacing-dispatch trap; if app feedback later wants re-raise-on-retry, that is a small follow-up.)

**Resolved decision (2026-09-03):** default remains `reconnect_attempts: 0`; the TCP connection still recovers on every subsequent call (`ensure_connected` is independent of this setting). Teams that want one automatic retry opt in explicitly with `Emb.configure { |c| c.reconnect_attempts = 1 }` — accepted tradeoff: a retry may duplicate work when the server had already processed the batch (e.g. slow replies), which is why it is not the default. Ideal future policy: retry only write-phase failures (command provably never sent); redis-client exposes no such granularity today.