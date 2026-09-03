# Design: Round-robin connection distribution for the Ruby client

## Context

See `proposal.md` — Why. The problem is two-fold: AWS Service Connect balances only at
connection establishment (requests on a keep-alive connection stay pinned to one
instance for its lifetime), and the gem's pool (`connection_pool` gem, `TimedStack`
with `@que.pop`) is **LIFO**, so at steady low concurrency one hot connection handles
everything → one hot emb instance. The fix works *with* Service Connect's granularity:
a pool of N persistent connections establishes on N different instances (Service
Connect round-robins connections), so routing each command through the *next* connection
spreads traffic across all instances — no service discovery, no IAM, no SG changes.

Requirements contract: `openspec/changes/ruby-client-round-robin/specs/` — rotation
order, pool-size-as-connection-count, thread safety, unchanged failure behavior.

## Goals / Non-Goals

**Goals**
- Replace LIFO reuse with round-robin connection selection for every command that goes
  through the client (`send_command` is the single funnel: `EMB`, `EMB.MULTI`, `INFO`,
  `PING`, proxy calls, and `Emb::Batch` chunks).
- Keep `pool:` meaning "number of connections" and keep all external behaviors that
  users currently rely on (thread safety, `reconnect_attempts`, error propagation,
  `Emb.debug?` timing output, `client.pool` accessor shape).
- Full test coverage of rotation, concurrency, and compat.

**Non-Goals**
- Load-aware / least-loaded routing (the B+ variant), Cloud Map discovery, per-instance
  probing, `ERR busy` handling changes.
- Any server-side change; any change to default `pool` size (stays 5); any config
  default flip.
- Cache-affinity or stickiness policies (unique-text workloads don't need them).

## Decisions

### 1. Custom ~60-line dispatcher instead of forking/monkey-patching `connection_pool`

`ConnectionPool` hardcodes `TimedStack` in `initialize` — there is no injection hook,
and its LIFO `fetch_connection` (`@que.pop`) is exactly the behavior we must replace.
Options considered:
- *FIFO via monkey-patching `TimedStack`* — fragile, global side effects, and still not
  true round-robin (FIFO checkout breaks the wrap-around ordering contract).
- *Fork `connection_pool`* — disproportionate for one method.
- **Chosen: a small dispatcher owned by the gem** that holds N `RedisClient` instances
  and a rotating index. Self-contained, unit-testable with a fake, no gem surgery.

To limit API breakage, the dispatcher exposes `size` and `with { |conn| }` mirroring
`ConnectionPool`'s surface used by the client (and `client.pool` keeps working for
readers).

### 2. Create RedisClient objects eagerly, connect lazily

Pool construction creates N `RedisClient` objects up front (no I/O — cheap), and each
establishes its TCP connection on first use (redis-client's default). Rationale: eager
`.connect` on `Emb.new` would make client construction fail when the server is briefly
down at boot (breaks apps that build the client before the server is reachable).
Consequence: with a cold client, the first N commands each open a connection and Service
Connect places them on N distinct instances — spread is complete within the first N
commands. No warm-up ping (YAGNI; can be a config knob later if boot-spread matters).

### 3. Per-connection mutex for thread safety

Concurrency model: a global mutex guards the rotating index; each connection owns a
mutex so a `RedisClient` (not thread-safe per instance) is only ever used by one caller
at a time. Up to N commands run in parallel; beyond that they serialize on connection
mutexes until a connection frees — there is **no checkout timeout** (the previous
`ConnectionPool` raised `TimeoutError` after 5s; Ruby `Mutex` offers no timed lock, and
for token-budget batching waiting is the desired behavior). Reentrant from the same
thread: a nested `with` re-enters the connection already held, mirroring
`connection_pool`'s thread-keyed checkout, so documented patterns like
`client.pool.with { |conn| conn.pipelined { … } }` keep working.
Alternatives rejected: checkout/checkin stack (equivalent semantics, more bookkeeping)
and a per-dispatch lock-free scheme (unsafe — RedisClient is not thread-safe).

### 4. Reconnect and error behavior delegating to redis-client, unchanged

Connection drops (scale-in, deploy, restart) are handled exactly as today: redis-client
reconnects within `call` per `reconnect_attempts`, and after retries fail the error
propagates to the caller. The dispatcher adds no health tracking — a dead connection
recovers on its next use, and after reconnect Service Connect places it (possibly on a
new instance), which additionally rebalances the pool over time.

### 5. Rotation granularity: per command (per `send_command`)

The batcher already calls `send_command` once per `batch_size` chunk, so chunk-level
spread for `Emb::Batch` and request-level spread for eager calls both fall out of a
single change point with no new code paths.

### 6. Fork safety preserved

The previous `connection_pool` engine closed and rebuilt all connections in the
child after `fork()` (`auto_reload_after_fork: true`, via a `Process._fork` hook);
`RoundRobinPool` mirrors that — pools register in an `ObjectSpace::WeakMap`, a
`Process._fork` prepend closes inherited sockets in the child (never sharing a
keep-alive connection between parent and child, which would interleave RESP
replies) and rebuilds index/mutexes (which would otherwise be inherited locked).
This also covers `inherit_socket: true`, where redis-client's own PIDCache socket
close is skipped. No-op where `Process.fork` is unavailable.

### 7. `connection_pool` gem dependency

`client.rb` is the only consumer (verified by grep); if nothing else in `gems/emb`
(including specs) requires it, drop it from the gemspec runtime dependencies. Defer the
final call to implementation time after a fresh grep.

## Risks / Trade-offs

- **[SC spread capped at pool size]** — if emb scales beyond `pool`, extra instances
  sit idle until reconnect cycles land connections on them. → Document `pool ≥ expected
  instance count` in the README; reconnect cycles (scale events) rebalance over time.
- **[Tail serialization when pool < concurrency — no checkout timeout]** — unlike
the previous `ConnectionPool` (5s wait then `ConnectionPool::TimeoutError`),
commands block until a connection frees: Ruby `Mutex` offers no timed lock, and
blocking suits token-budget batching (ride out the queue vs erroring). Deliberate;
the README documents it. Mitigation: size the pool to expected concurrency and cap
`read_timeout` if a hung instance must fail fast.
- **[Instance-scoped commands rotate]** — `INFO`/`EMB.STATS`/`CONFIG GET/SET` are
per-instance; rotation means each call samples (and `CONFIG SET` mutates) a random
instance. Documented in the README; cluster-wide aggregation is out of scope here
(future work).
- **[Boot spread ramps over first N commands]** (lazy connect) — a freshly booted
  web instance's first N embeddings go to N instances rather than all at once. →
  Negligible (N = pool size, default 5); a future `warm:` option can pre-connect.
- **[`client.pool` type change]** — previously a `ConnectionPool` instance. → Mirror
  `size`/`with`; document that internals are gem-owned now.

## Migration Plan

- **Deploy**: ship as a normal gem release (`just validate-gems` + release flow). No
  server-side change, no user config required, single-instance dev behavior identical.
- **Rollback**: pin the previous gem version — the dispatcher is confined to
  `gems/emb`, nothing else observes it.
- **Docs**: README "Connections" section — explain connection-level balancing (Service
  Connect / NLB) and recommend `pool: <emb instance count>`.

## Open Questions

- Whether to also accept `connections:` as an alias for `pool:` in `Emb::Client`
  (cosmetic naming; no spec impact).
- Whether to drop the `connection_pool` gemspec dependency (resolved at implementation
  time by grep; no spec impact).