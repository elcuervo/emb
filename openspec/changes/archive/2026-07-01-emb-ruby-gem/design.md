## Context

A Ruby gem that wraps `emb`'s Redis protocol. No native extensions — purely a network client. Follows the structure and conventions of the sibling `gte` gem.

## Goals / Non-Goals

**Goals:**
- `Emb[:model]["text"]` syntax with memoized proxies
- All server commands exposed: EMB, EMB.MULTI, EMB.MODELS, EMB.INFO, EMB.STATS, EMB.HELP, PING
- Thin Redis protocol wrapper via `redis-client` + `connection_pool` gems
- Nix dev shell for development
- Follow gte gem structure: gemspec, Gemfile, VERSION, lib/

**Non-Goals:**
- Native C/Rust extensions (all network calls)
- Auto-discovery of server models
- YAML or file-based configuration
- Client-side model config storage

## Decisions

### Decision: Module-level API with memoized registry

```
Emb.setup(host: "localhost", port: 6379, pool: 5)
  → sets class-level config, creates ConnectionPool

Emb[:minilm]
  → Emb::Proxy.new(:minilm) — cached in @registry

Emb[:minilm]["hello"]
  → proxy calls pool.with { |r| r.call("EMB", :minilm, "hello") }
  → returns raw bytes (float32 little-endian)

Emb.models
  → pool.with { |r| r.call("EMB.MODELS") }
  → parses RESP array into structured result
```

### Decision: Connection flow

```
Emb.setup(host: "localhost", port: 6379, pool: 5)
  → creates ConnectionPool wrapping RedisClient

Each pooled connection is a RedisClient instance.
Pool is created lazily on first command, then reused.
```

### Decision: Multi-model batch

`Emb.multi` uses a block-scoped proxy that queues commands and sends them atomically:

```
Emb.multi do |m|
  m[:minilm]["hello"]     # queues [:minilm, "hello"]
  m[:bge]["world"]        # queues [:bge, "world"]
end
# → EMB.MULTI minilm "hello" bge "world"
```

The multi proxy collects pairs, then flushes on block exit.

### Decision: Return types

- `Emb[:model]["text"]` → raw binary String (float32 bytes)
- `Emb.models` → Array of `{name:, dim:, status:}` hashes
- `Emb.info(name)` → Hash of key-value pairs
- `Emb.stats` → Hash of key-value pairs
- `Emb.ping` → `"PONG"`
- `Emb.help` → String

### Decision: File structure

```
gem/
  emb.gemspec
  Gemfile
  VERSION
  lib/
    emb.rb
    emb/
      version.rb
      client.rb      # connection pool + raw send
      proxy.rb       # Emb[:model] handler
      multi.rb       # block-scoped batch proxy
```

## Risks / Trade-offs

- **No fallback on connection failure** — errors bubble up as RedisClient exceptions. Mitigation: configure `reconnect_attempts` on RedisClient.
- **Pool exhaustion** — if all connections are busy, `ConnectionPool` blocks. Mitigation: reasonable default pool size (5), configurable.
- **Emb[:model] is global** — registry is module-level, shared across threads. Mitigation: registry is just a Hash, thread-safe for reads; writes happen once per model name.
