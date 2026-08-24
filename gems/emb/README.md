# emb — Ruby client

[![emb gem](https://img.shields.io/gem/v/emb?logo=rubygems&color=red&label=emb)](https://rubygems.org/gems/emb)

Thin Ruby wrapper for [emb](https://github.com/elcuervo/emb), a Redis-compatible embedding server. Auto-decodes float32 binary responses to Ruby arrays.

## Installation

Add to your Gemfile:

```ruby
gem "emb"
```

Or install globally:

```bash
gem install emb
```

## Setup

The client connects to an emb server via the Redis protocol (RESP2). Configure with a URL,
host/port, or rely on defaults and environment variables:

```ruby
require "emb"

# URL (Redis URL format)
Emb.setup(url: "redis://localhost:6379")

# Or individual params
Emb.setup(host: "localhost", port: 6379)

# Or rely on defaults
Emb.setup
```

`Emb.config` is an alias for `Emb.setup`.

### Configuration sources (priority order)

1. Explicit `url:` or `host:`/`port:` arguments
2. `EMB_URL` environment variable
3. Default: `redis://localhost:6379`

### Global configuration

Every client (`Emb.setup`, `Emb.new`, and the lazily-created default client) inherits a
shared global `Emb::Config` value object set with `Emb.configure`. Settings
resolve in this order — the first one wins:

1. **Explicit per-call option** (`Emb.setup(pool: 10)`)
2. **`Emb.configure` value**
3. **Built-in default**

```ruby
Emb.configure do |c|
  c.pool = 8
  c.batch = false   # opt out of lazy batching app-wide
end

Emb.configuration   # => the shared Emb::Configuration
Emb::Client.new(pool: 20)  # per-call still wins
```

`EMB_URL` remains the only environment variable (connection URL fallback, as before);
`Emb.configure { |c| c.url = ... }` or an explicit `url:` override it.

### Out-of-the-box defaults

The shipped defaults are benchmark-derived (see `BENCHMARK.md`): **lazy batching is on by
default** (`batch: true` — each embed coalesces into one `EMB.MULTI`), pool `5`, pure-Ruby
RESP driver, `protocol: 2`, `reconnect_attempts: 3`. To keep the eager behavior (immediate
`EMB` per call), opt out globally via `Emb.configure { |c| c.batch = false }` or per client
with `Emb.new(batch: false)`.

### Connection pool

```ruby
Emb.setup(url: "redis://localhost:6379", pool: 10)
```

The default pool size is **5**. The pool is usually not the bottleneck for
inference-bound workloads (small pools are fine); it becomes a knob only at high
concurrency on a multi-model box — see [Performance](#performance). If a pool checkout
would wait too long, `RedisClient`'s `connect_timeout`/`read_timeout` (above) bound the
wait.

### Authentication

If the server is configured with a password, include it in the URL:

```ruby
# Password as URL userinfo
Emb.setup(url: "redis://:hunter2@localhost:6379")
```

The `RedisClient` gem handles `AUTH` automatically on connect when a password
is embedded in the URL. This works correctly with connection pooling — every
connection in the pool authenticates on creation.

Manual authentication is also possible but not recommended for pooled connections:

```ruby
Emb.send_command("AUTH", "hunter2")  # only authenticates one connection
```

### Redis client options

Any `RedisClient` option can be forwarded through `Emb.setup` or `Emb.new`:

```ruby
Emb.setup(
  url: "redis://localhost:6379",
  pool: 10,
  connect_timeout: 2,
  read_timeout: 10,
  write_timeout: 5,
  reconnect_attempts: 5,
  ssl: true,
  ssl_params: { verify_mode: OpenSSL::SSL::VERIFY_PEER },
  driver: :hiredis,
  inherit_socket: true
)
```

See the [redis-client documentation](https://github.com/redis-rb/redis-client) for
all available options. Only `pool` and `batch` are handled by the gem — everything
else passes through to `RedisClient.new`.

## Instance-based clients

Create independent clients to connect to multiple servers or use different configurations:

```ruby
default = Emb.setup(url: "redis://localhost:6379")
other   = Emb.new(url: "redis://:hunter2@10.0.0.1:6380")

default.ping  # => "PONG"
other.ping    # => "PONG"
```

Each client has its own connection pool and model proxy registry:

```ruby
c1 = Emb.new(url: "redis://server1:6379")
c2 = Emb.new(url: "redis://server2:6379")

c1[:minilm] != c2[:minilm]  # separate proxies
```

### Global convenience API

When you don't need multiple clients, use the module-level methods:

```ruby
Emb.setup

Emb[:minilm]["hello"]   # proxy access
Emb.models               # list models
Emb.info(:minilm)        # model info
Emb.stats                # server stats
Emb.help                 # command reference
Emb.ping                 # health check
```

These all delegate to a lazily-initialized default client. No explicit `setup` call
is required for simple cases — the default client connects to `redis://localhost:6379`
automatically.

## Usage

### Single text

```ruby
result = Emb[:minilm]["hello world"]
# => [0.0123, -0.0456, 0.0789, ...]  (384 floats)
```

With an instance-based client:

```ruby
client = Emb.new(url: "redis://localhost:6379")
result = client[:minilm]["hello world"]
```

### Multiple texts

```ruby
results = Emb[:minilm]["hello", "world"]
# => [[0.0123, ...], [-0.0456, ...]]
```

### Multi-model queries

Send texts to different models in one round trip:

```ruby
results = Emb.multi do |m|
  m[:minilm]["hello"]
  m[:bge]["world"]
end
# => [[0.0123, ...], [-0.0456, ...]]
# Results are unpacked from float32 binary — same format as single embeddings
```

Works the same on instance clients:

```ruby
client.multi do |m|
  m[:minilm]["hello"]
  m[:bge]["world"]
end
```

### Lazy batching (`Emb.batch`)

Instead of collecting pairs by hand, `Emb.batch` returns lazy embeddings that all
coalesce into a single `EMB.MULTI` round trip when the first one is used. This is
powered by the [batch-loader](https://github.com/exAspArk/batch-loader) gem.

```ruby
users = User.all # some application objects

# Create loaders first...
l1 = Emb.batch[:minilm]["hello"]
l2 = Emb.batch[:minilm]["world"]
l3 = Emb.batch[:bge]["bonjour"]

# ...then consume them. The first use sends ONE EMB.MULTI for all three.
l1.sum  # => 12.345
l2.sum  # => -0.678
l3.sum  # => 3.141
```

Instance clients expose the same API:

```ruby
client.batch[:minilm]["hello"].sum
```

Each lazy value materializes to the same shape as the eager API: a single text
yields an `Array<Float>`, multiple texts yield `Array<Array<Float>>`:

```ruby
vec   = Emb.batch[:minilm]["hello"]   # use -> Array of Float
vecs  = Emb.batch[:minilm]["hello", "world"]  # use -> Array of Array of Float
```

Embeddings are cached per thread, so reusing a lazy value (or creating an
identical pair again in the same scope) is free after the first use. A pair whose
embedding fails materializes as `nil`, matching `EMB.MULTI`'s per-pair null
behavior; siblings in the same batch still succeed.

#### The create-then-consume contract

Loaders only fire when a value is **used**. Create all loaders *first*, then
consume them, so they share one round trip:

```ruby
texts.each { |t| process(Emb.batch[:minilm][t]) }   # wrong: one MULTI per item
loaders = texts.map { |t| Emb.batch[:minilm][t] }   # right: ONE MULTI for all
loaders.each { |l| process(l) }
```

A loader that is created but never used **never embeds** (unless a sibling batch
fires first) and is silently dropped when the thread's scope ends. Duration and
scope: batching is per-thread — a multithreaded app issues one `EMB.MULTI` per
thread per flush.

### `batch` configuration option

Setting `batch: true` makes the standard proxy API lazy, so existing call sites
batch automatically without restructuring:

```ruby
Emb.setup(url: "redis://localhost:6379", batch: true)
# or
Emb.new(url: "redis://localhost:6379", batch: true)
```

With `batch: true`, `Emb[:minilm]["hello"]` returns a lazy embedding that sends
`EMB.MULTI` on first use. The default is `false` — the proxy API stays eager,
sending `EMB` immediately. `Emb.batch` works regardless of the option, and
`Emb.multi` remains the explicit, eager, deterministic batching API.

### Clearing the cache per request

The per-thread batch scope holds cached embeddings for the life of the thread. In
request-shaped processes (Rails, Rack apps, Sidekiq) mount `Emb::Middleware` to
clear the scope at the end of each request:

```ruby
# config/application.rb (Rails)
config.middleware.use Emb::Middleware

# Any Rack app
use Emb::Middleware
```

The scope is cleared even when the app raises, and a fresh scope starts
automatically with the next request. Loaders created but never used within a
request are dropped — the create-then-consume contract applies per request.

## Performance

### Eager-burst pipelining (no new API)

For a burst of independent eager calls (feelers across call sites, fan-ins where
batching doesn't fit), coalesce them into one packet with `RedisClient#pipelined`:

```ruby
client = Emb.new(url: "redis://localhost:6379")

a = client.pool.with do |conn|
  conn.pipelined do |pipe|
    texts.each { |t| pipe.call("EMB", "minilm", t) }
  end
end
# a -> Array of Float32-binary replies; unpack each with r.unpack("e*")
```

`Client#pool` exposes the pool's `RedisClient` connections. This measurably beats
plain per-call round trips; use it for bursts, and `Emb.batch`/`Emb.multi` when you want
server-side coalescing into one `EMB.MULTI`.

### RESP driver: pure-Ruby default, `hiredis` on demand

The pure-Ruby RESP parser is the default. The C `hiredis` driver only meaningfully helps
the all-round-trip eager path (about +12% req/s) and is ~neutral for batched/pipelined
workloads, so it is not worth the native-build dependency by default. Enable it when
round-trip-heavy eager traffic dominates:

```ruby
require "hiredis-client"
Emb.setup(url: "redis://localhost:6379", driver: :hiredis)
```

### Horizontal scaling

emb instances are stateless — the model lives in memory and the LRU cache is per
instance. Scale out without a cluster client:

- **Model sharding** — run each instance with a subset of models; route by model via
  dedicated clients (`Emb.new(url: "redis://model-a:6379")`).
- **Text-keyed sharding** — several instances serving the same model behind an L4 /
  load balancer for same-model scale.
- **Cache warm-up** — because the cache is per instance, each new box starts cold
  (optionally seed with `-cache` and a warmup pass).

Lazy batching stays per server (a thread's loaders target one client), so batching and
horizontal scale compose: each instance receives one `EMB.MULTI` per request.

### Commands

```ruby
Emb.models   # => [{name: "minilm", dim: 384, status: "ready"}, ...]
Emb.info(:minilm)  # => {dim: 384, workers: 10, requests: 42, ...}
Emb.stats    # => server statistics hash
Emb.help     # => command reference string
Emb.ping     # => "PONG"
```

## Development

### Console

Start an IRB session with the gem loaded:

```bash
bundle exec rake console
```

### Lint

```bash
bundle exec rubocop
```

### Tests

Start the emb server, then run the test suite:

```bash
# From the repo root:
./bin/emb -config test-two-models.yaml &

# From gems/emb/:
bundle exec rake
```

Tests cover all commands: `EMB`, `EMB.MODELS`, `EMB.INFO`, `EMB.HELP`, `PING`,
and `EMB.MULTI`, plus instance-based clients, URL configuration, and connection pooling.
