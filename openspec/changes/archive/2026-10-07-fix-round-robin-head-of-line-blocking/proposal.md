# Proposal

## Why

The 0.4.x `RoundRobinPool` assigns each command a fixed connection index and then blocks
on that connection's mutex. When more commands are in flight than pool connections, a
command can wait for a busy connection while another connection is free — head-of-line
blocking. This is the ~150 ms p90 regression observed on `emb`-backed endpoints versus
0.3.0, whose `connection_pool` pool woke a waiter when *any* connection was returned. The
change must remove the blocking while keeping the connection-level load-balancer fan-out
that motivated round-robin in the first place.

## What Changes

- **Work-conserving connection selection** in `Emb::RoundRobinPool`: a command takes the
  next *available* connection (FIFO), never waiting behind a busy connection while another
  is free. Idle/sequential rotation and multi-instance ordering are preserved, so
  connection-level load-balancer fan-out is unchanged.
- **Committed deterministic regression guard**: a gated-fake spec proving a waiting
  command runs on a freed connection without waiting for a slow one (fails against the
  current pool, passes after the fix). No wall-clock assertions.
- **Tail-latency benchmark gate**: a concurrent pool scenario in the gem benchmark
  harness with a documented p90 threshold against the 0.3.0 (work-conserving) baseline,
  so a future pool change cannot silently reintroduce the tail.
- **Docs**: README connection-pool section and `BENCHMARK.md` record the work-conserving
  behavior, why unbounded queueing is still preferred over the old 5s checkout timeout,
  and the new gate.

## Capabilities

### New Capabilities

<!-- none: this hardens the existing pool behavior; no new capability is introduced. -->

### Modified Capabilities

- `ruby-client-round-robin`: selection becomes work-conserving — a command SHALL NOT wait
  for a busy connection while another pool connection is free; sequential rotation and
  multi-instance ordering stay as specified.
- `ruby-client-benchmarks`: the harness SHALL include a concurrent pool tail-latency
  scenario with a documented p90 threshold and SHALL fail the gate above it.

## Impact

- **Gem code**: `gems/emb/lib/emb/round_robin_pool.rb` (selection, fork reset).
- **Gem specs**: `gems/emb/spec/emb/round_robin_pool_spec.rb` (new work-conservation
  scenario; existing rotation/concurrency/fork specs unchanged).
- **Bench harness/docs**: `gems/emb/bench/bench.rb`, `justfile` (`bench-ruby`),
  `BENCHMARK.md`, `gems/emb/README.md`.
- **No API, wire, config, or dependency change** — patch-level release.
