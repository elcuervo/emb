# Design

## Context

See proposal.md — Why. `Emb::RoundRobinPool` (introduced in the 0.4.0 window, replacing
the `connection_pool` gem) selects `@next % size` and then blocks on that single
connection's mutex:

```ruby
def pick
  @index_mutex.synchronize { idx = @next % @size; @next += 1; [idx, @connections[idx]] }
end

def take(held)
  idx, connection = pick
  held[self] = idx
  @locks[idx].synchronize { yield connection }   # waits for THIS connection
end
```

`connection_pool`'s `TimedStack` instead parks waiters on a condition variable that any
`push` signals, so it is work-conserving. The fixed-index wait is the only request-path
difference that produces a p90-only tail (p50 flat, one slow service time moved into p90).

Measured with a micro-harness (5 connections, 10 threads, 5% of replies 150 ms):

| pool | p50 | p90 | p99 |
|---|---|---|---|
| `connection_pool` (0.3.0) | 7.5 ms | 7.6 ms | 160 ms |
| `RoundRobinPool` (0.4.x) | 13.0 ms | **157.0 ms** | 196 ms |
| work-conserving prototype | 7.5 ms | 7.6 ms | 160 ms |

## Goals / Non-Goals

**Goals:**
- Remove head-of-line blocking: a waiting command acquires the first connection that
  frees.
- Preserve idle rotation and multi-instance ordering (the Service Connect / NLB
  connection-level fan-out rationale is unchanged).
- Keep the existing public surface (`size`, `connections`, reentrancy, fork reset) and
  the unbounded wait (no reintroduction of a 5s checkout timeout).

**Non-Goals:**
- Load-aware / least-loaded routing, health tracking, or per-instance probing.
- Changing the `lazy` execution default or any other 0.4.0 behavior.
- Reverting to the `connection_pool` gem (its LIFO reuse is what round-robin replaced).

## Decisions

### D1 — Free-connection FIFO queue instead of fixed-index mutex wait

Replace `@locks` / `@next` / `@index_mutex` / `pick` with a `Queue` of free connection
indices. `with` pops an index, yields, and pushes it back in `ensure`. Because a released
connection is pushed to the tail, a single-threaded caller still sees strict rotation
(`0,1,…,N-1,0,…`); under concurrency the pop is work-conserving.

```ruby
def initialize(size, &blk)
  @size = size
  @connections = Array.new(size, &blk)
  @free = Queue.new
  size.times { |i| @free << i }
  INSTANCES&.[]=(self, self)
end

def with(&)
  held = Thread.current[THREAD_KEY]
  return yield @connections[held[self]] if held&.key?(self)

  idx = @free.pop
  held ||= {}
  Thread.current[THREAD_KEY] = held
  held[self] = idx
  begin
    yield @connections[idx]
  ensure
    held.delete(self)
    Thread.current[THREAD_KEY] = nil if held.empty?
    @free << idx
  end
end

def reload_after_fork!
  @connections.each { |c| c.close if c.respond_to?(:close) }
  @free = Queue.new
  @size.times { |i| @free << i }
end
```

Alternatives considered:
- **`try_lock` scan from the rotating index, block on the next in rotation if all busy** —
  equivalent work-conserving behavior, but more code and it keeps `@locks`. Rejected.
- **Reuse `connection_pool`** — its LIFO stack pins traffic to one connection, defeating
  the connection-level fan-out. Rejected.
- **Add a checkout timeout** — bounds the wait but does not remove the blocking, and
  reintroduces `TimeoutError` under saturation. Rejected.

The queue *is* the mutual exclusion (an index is only handed to one caller at a time), so
the per-connection mutexes are no longer needed.

### D2 — Deterministic regression guard, not a timing gate

The gem spec uses gated fakes (the same pattern as `internal/pipeline/batcher_budget_test.go`)
to pin work-conservation without wall-clock assertions:

1. `pool: 2`; start A on connection 0 and B on connection 1, each blocked on its own gate.
2. Start C: it must be waiting (both connections held).
3. Release B → the harness waits until C completes, while A is still held.
4. Assert C ran on connection 1 before A's gate was released, then release A.

Against the current pool, C picked index 0 and stays blocked on connection 0, so the
assertion times out and fails. Against the fix, C is served from the freed connection.

### D3 — Keep the benchmark gate model-free, beside the client-timeout repro

Injecting slow replies against a live emb server is not possible, so the gate lives
in `bench/repro/pool-hol/`: a mock RESP2 server answers `EMB`/`EMB.MULTI` with real
float32 bulks and a configurable slow fraction, and `gate.rb` drives the real client
from more threads than pool connections and fails when p90 reaches the slow path.
`just bench-pool-gate` runs it; the deterministic spec, not a second pool
implementation, proves fixed-index selection fails. It reuses
`bench/repro/client-timeout/mock_server.rb` (extended with a slow-reply fraction)
and needs no model or emb build.

## Risks / Trade-offs

- [Removing per-connection mutexes could let two threads share a connection if the queue
  is misused] → The queue holds each index exactly once; `reload_after_fork!` rebuilds it,
  and the existing "never used from two threads at once" spec stays green.
- [FIFO reuse could differ from the previous *hot-connection* behavior] → The 0.4.0
  behavior was already round-robin (strict rotation when idle); FIFO preserves it. There
  is no behavior to preserve from the 0.4.x fixed-index path other than the bug.
- [Forked children could inherit a queue missing checked-out indices] → Rebuild `@free`
  with every index in `reload_after_fork!`.
- [A patched pool still blocks when all connections are busy] → Intended; `pool` is the
  concurrency budget, and the README already documents sizing it to expected concurrency.

## Migration Plan

1. Land the pool change + deterministic spec + benchmark scenario/threshold + docs.
2. Run `just test`, the gem spec suite, and `just bench-ruby` to confirm p90 parity with
   the recorded work-conserving baseline.
3. Release as a patch; no user action required, no config or API change.

## Open Questions

None blocking. Whether to also flip the client's default back to coalescing (`lazy:
:multi`) is a separate decision about the 0.4.0 eager default, not this fix.
