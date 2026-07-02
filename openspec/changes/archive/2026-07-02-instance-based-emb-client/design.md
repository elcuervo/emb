# Design: Instance-based Emb Client

## Architecture

```
emb.rb  (module entry point)
├── Emb.setup / Emb.config → creates default Emb::Client
├── Emb.new → returns new Emb::Client
├── Emb.[], .models, .info, .stats, .help, .ping, .multi → delegate to default client
├── Emb.send_command → delegate to default client.send_command
│
emb/client.rb
└── Emb::Client
    ├── initialize(url: nil, host: nil, port: nil, pool: 5)
    │   URL resolution:
    │     url arg → host+port → ENV["EMB_URL"] → "redis://localhost:6379"
    │
    ├── @pool → ConnectionPool { RedisClient.new(url: ..., protocol: 2, reconnect_attempts: 3) }
    ├── @registry → {} (model name → Proxy)
    ├── send_command(*args) → @pool.with { |r| r.call(*args) }
    ├── [](name) → @registry[name] ||= Proxy.new(self, name)
    ├── models → send_command("EMB.MODELS") → parse [{name:, dim:, status:}, ...]
    ├── info(name) → send_command("EMB.INFO", name) → parse {...}
    ├── stats → send_command("EMB.STATS")
    ├── help → send_command("EMB.HELP")
    ├── ping → send_command("PING")
    └── multi(&block) → MultiProxy → send_command("EMB.MULTI", ...) → unpack each
│
emb/proxy.rb
└── Emb::Proxy
    ├── initialize(client, name)
    │   @client = client, @name = name
    └── [](text, *texts)
        → @client.send_command("EMB", @name, text, *texts)
        → Array(...).map { |entry| entry.unpack("e*") }
        → single result returns Array<Float>, multi returns Array<Array<Float>>
│
emb/multi.rb
└── Emb::MultiProxy
    ├── initialize(client)
    │   @client = client, @pairs = []
    ├── [](name) → PairCollector
    └── run
        → args = @pairs.flat_map { |p| [p[:model], p[:text]] }
        → raw = @client.send_command("EMB.MULTI", *args)
        → raw.map { |entry| entry.unpack("e*") }
```

## URL Resolution Priority

```
Emb.new()
  │
  ├── url: "redis://..." ────────────→ use directly
  ├── host: "x" + port: 6379 ────────→ "redis://x:6379"
  ├── EMB_URL env var ────────────────→ read from env
  └── nothing ────────────────────────→ "redis://localhost:6379"
```

## Backward Compatibility

| Old | New |
|-----|-----|
| `Emb.setup(host:, port:, pool:)` | Still works, constructs URL internally |
| `Emb.ping` | Delegates to `default_client.ping` |
| `Emb[:model]` | Delegates to `default_client[:model]` |
| `Emb.multi { ... }` | Delegates to `default_client.multi { ... }` |
| `Emb.models` | Delegates to `default_client.models` |

All old specs pass unchanged. New specs add test coverage for instance-based usage.

## Multi Unpack Fix

`EMB.MULTI` returns an array of bulk strings (float32 binary), same format as `EMB`.
The current `MultiProxy#run` returns raw RESP array. Fix: map each entry through
`unpack("e*")` — identical to what `Proxy#[]` does.

```ruby
# Before
def run
  args = @pairs.flat_map { |p| [p[:model].to_s, p[:text]] }
  Emb.send_command("EMB.MULTI", *args)
end

# After
def run
  args = @pairs.flat_map { |p| [p[:model].to_s, p[:text]] }
  raw = @client.send_command("EMB.MULTI", *args)
  raw.map { |entry| entry.unpack("e*") }
end
```

## Thread Safety

`Emb::Client` uses `ConnectionPool` which is thread-safe. The `@registry` hash
is written on `[]` calls — this is safe under CRuby GIL but could race with
`Thread::Mutex` if needed. Simple memoization is fine for the gem's use case.

The module-level default client (`@default_client`) uses lazy init under a Mutex
to avoid races on first access.
