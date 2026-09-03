## Why

Production showed sustained server CPU/network with no matching client success, dropping "back to normal" only when the web app restarted. A reproduced, fully validated mechanism (`bench/repro/client-timeout/`) explains it: the gem's embedding calls carry redis-client's **1s default read timeout**; a slower reply raises `ReadTimeoutError`, which is a `ConnectionError` subclass, so redis-client **auto-re-sends the whole EMB.MULTI up to `reconnect_attempts+1` times (gem default 3 → 4× server work)**; then the error propagates into `Emb::BATCH_BLOCK`, which **raises before batch-loader's `delete(items:)`** — every deferred item stays in the thread-local executor — so any app-level retry **re-forces the entire batch from scratch** (another 4×), a self-sustaining loop with zero client progress. The app restart merely wipes the thread-local state, masking the loop until the next >1s batch. Defaults must be secure (no silent re-send, no retained re-run) and growth must be impossible.

## What Changes

- **`gems/emb` config defaults** (secure by default, still overridable):
  - `read_timeout` / `write_timeout`: explicit **10s** instead of `nil` (redis-client then falls back to its silent 1.0s default that makes 512-pair batches fail under load).
  - `reconnect_attempts`: **0** instead of 3 — no automatic re-send of a non-idempotent `EMB.MULTI` (each resend duplicates server inference).
- **`Emb::BATCH_BLOCK` fails closed**: any batch command failure surfaces the error to the forcing caller **once** and **removes every deferred item from the scope's pending set** — retries cannot re-run the batch, later resolutions return `nil`, subsequent batches in the same scope contain only items deferred after the failure (no stale re-sends, no pending growth).
- **Committed evidence**: `bench/repro/client-timeout/{mock_server.rb,repro.rb,README.md}` — the validated reproduction (19 assertions) plus fix demos.
- **Docs**: README defaults table updated with the new defaults and the fail-closed batch semantics.

## Capabilities

### New Capabilities

<!-- none — this hardens existing gem behaviors (batched calls + configuration
     defaults); no new command or wire surface is introduced. -->

### Modified Capabilities
- `client-global-configuration`: out-of-the-box defaults change — explicit 10s read/write timeouts (previously `nil`, resolving to redis-client's 1s) and `reconnect_attempts: 0` (previously 3), while remaining overridable via `Emb.configure`.
- `ruby-batch-loading`: batch failures fail closed — the error surfaces once, the deferred set is cleared, re-resolution sends nothing and returns `nil`, and later batches exclude the failed items (no re-run storm, no pending accumulation).

## Impact

- **Code**: `gems/emb/lib/emb/configuration.rb` (defaults), `gems/emb/lib/emb/batch.rb` (fail-closed `BATCH_BLOCK` + pending-clear helper), README, gem specs (`spec/emb_spec.rb`, new batch-failure specs). No server changes.
- **New repo files**: `bench/repro/client-timeout/` (mock RESP2 server, repro driver, README — validation evidence, kept as a regression script).
- **Behavioral notes**: after a failed batch, re-resolution of the failed items returns `nil` (documented); app-level retries become the sole retry path (safe, since they now fail closed); a genuinely slow server surfaces errors at 10s instead of blocking indefinitely.
- **No wire changes**: RESP2 commands, INFO, EMB.STATS, and the existing per-pair null semantics for server-side partial failures are untouched.