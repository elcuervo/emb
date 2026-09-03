## Why

When emb is served behind AWS Service Connect, load balancing happens **at connection
establishment**: every request on a keep-alive connection rides to the same instance for
the lifetime of that connection. The Ruby client's pool is LIFO, so at steady low
concurrency it reuses one hot connection — pinning a web server's entire embedding
traffic to a single emb instance. When that instance's batcher backs up (token budget
queue), requests lock behind it while other instances sit idle. There is no load-aware
signal needed to fix this: since Service Connect spreads *connections* round-robin at
establishment, a pool of N persistent connections lands on N different instances, so
spreading commands across those connections spreads traffic across all instances.

## What Changes

- **Round-robin connection selection in `Emb::Client`**: replace the LIFO
  `connection_pool` reuse with a small round-robin dispatcher that routes every command
  (`send_command` — the single funnel for `EMB`, `EMB.MULTI`, `INFO`, `PING`, proxy
  calls, and batch chunks) through the *next* connection in the pool.
- **N connections, spread across instances**: `pool: N` yields N persistent Redis
  connections; through Service Connect these establish on N different instances, so
  traffic fans out across all of them at request (or batch-chunk) granularity — even
  for a single-threaded web process at zero concurrency.
- **Thread safety preserved**: concurrent calls remain safe (per-connection
  serialization); redis-client's existing reconnect behavior is untouched, so
  scale-in/drain reconnect cycles keep working unchanged.
- **Batch path benefits for free**: `Emb::Batch` already chunks `EMB.MULTI` at
  `batch_size` via `send_command`, so per-chunk rotation across instances comes without
  new code.
- **Docs**: README guidance that `pool` should be ≥ the emb instance count for full
  coverage behind Service Connect (or any connection-level load balancer).

## Capabilities

### New Capabilities
- `ruby-client-round-robin`: the client distributes commands across all pool
  connections in round-robin order — pool size controls connection count, repeated
  sends hit different connections, concurrent use stays safe, and failure/reconnect
  behavior is unchanged.

### Modified Capabilities
<!-- None: existing specs (`emb-ruby-client` "pool SHALL have size 10",
     `gem-redis-client-config` "only pool handled by the gem") remain true
     verbatim; no requirement changes. -->

## Impact

- **Gem**: `gems/emb/lib/emb/client.rb` (pool construction + `send_command`), likely a
  new small dispatcher file under `gems/emb/lib/emb/`, README connection section.
- **Specs**: new unit spec for the round-robin dispatcher (sequential rotation, pool
  size, concurrency safety, failure passthrough) plus existing client spec coverage.
- **Dependencies**: none new — replaces the internal `connection_pool` gem usage inside
  the client (gem may stay in the gemspec if other code uses it).
- **Behavioral note**: no config default changes; `pool` keeps its meaning (number of
  connections). Local single-instance dev behaves identically to today.