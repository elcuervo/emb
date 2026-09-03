# Client batch leak-loop guard (repro)

Reproduces the production-observed symptom and **guards the fix**: before
`secure-client-batch-defaults`, a slow server (reply > redis-client's silent
1.0s default timeout) caused `EMB.MULTI` to be auto-re-sent up to
`reconnect_attempts + 1` times (gem default 3 → 4× server work), and
batch-loader retained every deferred item when the batch block raised — so any
app retry re-forced the whole batch forever (sustained CPU/network until the
app restarted, which only wiped the thread-local state). The validated
original reproduction (with the pre-fix gem) is described in the git history of
this directory.

After the fix the gem ships **10s read/write timeouts**, **`reconnect_attempts: 0`**
(no automatic re-send of the non-idempotent `EMB.MULTI`), and a **fail-closed**
`Emb::BATCH_BLOCK` (error surfaces once; pending set cleared; retries inert and
resolve to `[]`; later batches exclude failed items).

## Run

```bash
cd gems/emb && bundle exec ruby ../bench/repro/client-timeout/repro.rb
```

(inside `nix develop`; ~15s; `MOCK_DELAY` env overrides mock inference time,
default `1.5` — keep it > 1s so Phase A's 1s-timeout client fails.)

## What it guards (all assertions hold with the hardened gem)

| Phase | Guards |
|---|---|
| **A — worst-case config** (1s timeout + 3 auto-resends, explicitly opted in) | the error surfaces once; the redis-client resend amplifier is visible but **contained** (pending set cleared → the retention loop cannot form); retries send nothing and resolve to `[]`; a subsequent batch in the same scope carries **only its own items** (stale excluded); pending stays 0 across repeated failures |
| **B — default 10s timeout** | one send, exact completion, memoized re-use — no loop |

If a future change reintroduces the retention loop (e.g. someone removes the
fail-closed rescue), Phase A fails immediately with "retries send NOTHING" or
"pending stays empty".

## Why reconnect_attempts defaults to 0

Recovery still happens: the TCP connection is re-established on every call
(`ensure_connected` is independent of this setting). What 0 disables is
re-sending the failed in-flight command — which duplicates non-idempotent
`EMB.MULTI` work. Teams that want one automatic retry opt in explicitly:
`Emb.configure { |c| c.reconnect_attempts = 1 }`.