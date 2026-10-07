# Pool head-of-line-blocking gate (repro)

Guards the 0.4.x p90 regression on `emb`-backed endpoints. The 0.4.0
`RoundRobinPool` assigned every command a fixed connection index and blocked on
that connection's mutex, so a waiting command could sit behind a slow inference
while another pool connection was free. `gems/emb` now selects the next
*available* connection; this gate fails if a future change reintroduces the
fixed-index wait.

## Run

```bash
just bench-pool-gate
```

Or directly (from `gems/emb`, so bundler resolves the gem):

```bash
cd gems/emb && bundle exec ruby ../../bench/repro/pool-hol/gate.rb
```

Model-free: it drives the real client against the shared repro mock
(`bench/repro/client-timeout/mock_server.rb`), which answers `EMB`/`EMB.MULTI`
with real float32 bulks and a configurable slow fraction, so no emb server or
downloaded model is required.

## What it measures

`gate.rb` runs `THREADS` concurrent eager embeds through a `pool: POOL` client
(`THREADS > POOL`, default 10 > 5) against the mock. `MOCK_SLOW_P` of replies
take `MOCK_SLOW` seconds (default 5% @ 150 ms); the rest take `MOCK_BASE`
(5 ms). It reports p50/p90/p99 and fails when p90 exceeds `MAX_P90_MS`.

Reference measurements (5 connections, 10 threads, 3000 calls):

| selection | p50 | p90 | p99 | gate |
|---|---|---|---|---|
| work-conserving (0.3.0-equivalent) | 7.6 ms | **7.8 ms** | 160 ms | PASS |
| legacy fixed-index (0.4.x) | 7.8 ms | **160.3 ms** | 334 ms | FAIL |

The slow reply is 150 ms, so the fixed-index p90 lands on the slow path while the
work-conserving p90 stays on the fast path — the threshold (50 ms) separates them
with headroom. The legacy row is the pre-fix pool
(`git show 32ddcba:gems/emb/lib/emb/round_robin_pool.rb`). Override with `MAX_P90_MS`, `MOCK_BASE`,
`MOCK_SLOW`, `MOCK_SLOW_P`, `POOL`, `THREADS`, `ITERS`.
