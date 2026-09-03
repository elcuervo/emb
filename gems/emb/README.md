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

The default pool size is **5**. The client keeps `pool` persistent connections and
routes commands through them **in round-robin order** — consecutive commands use
different connections instead of reusing one hot connection.

This matters when emb runs as a pool of instances behind a **connection-level load
balancer** (AWS Service Connect, an NLB, or Envoy): the balancer assigns each
keep-alive connection to exactly one instance for its lifetime, so relying on a
single connection would pin all of a process's embedding traffic to one instance —
and queue it there even while other instances sit idle. Round-robin spreading across
`pool` connections spreads traffic across up to `pool` instances, so set `pool` to
**at least your expected emb instance count** (e.g. `pool: 10` for 10 instances).

Commands beyond the pool's parallelism do **not** time out: when `pool` commands are
already in flight, later commands wait on the shared connections until one frees
(there is no checkout timeout — unlike the previous pool's 5s
`ConnectionPool::TimeoutError`). For inference workloads this lets commands ride out
token-budget batching instead of erroring.

Locally (a single server) any pool size behaves identically: every command simply
rounds through the same connections. The pool is usually not the bottleneck for
inference-bound workloads (small pools are fine); it becomes a knob at high
concurrency on a multi-model box — see [Performance](#performance). With
round-robin selection up to `pool` commands run in parallel; beyond that, commands
share connections and serialize on them.

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
all available options. Only `pool`, `batch`, and `batch_size` are handled by the
gem — everything else passes through to `RedisClient.new`.

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
Emb.stats                # server stats (Hash of key => value)
Emb.help                 # command reference
Emb.ping                 # health check
```

These all delegate to a lazily-initialized default client. No explicit `setup` call
is required for simple cases — the default client connects to `redis://localhost:6379`
automatically.

## Server info & config

> **Per-instance data.** `Emb.server_info`, `stats`, `info`, and `config` read
> from ONE instance, and with round-robin connection selection that instance
> changes between calls — each call is a sample. `Emb.config[key] = value`
> mutates one (arbitrary) instance. Use per-connection queries if you need a
> specific or aggregated view.

The server exposes Redis-style `INFO` and `CONFIG` commands; the gem wraps them.

### `Emb.stats` — server statistics as a hash

`EMB.STATS` is decoded into a Symbol-keyed Hash with values as the server sent them
(RESP integers stay Integer, everything else String):

```ruby
Emb.stats
# => {uptime_secs: 3, total_requests: 0, active_requests: "0", total_tokens: 0,
#     total_errors: 0, models_loaded: 1, per_model: "minilm: req=0 avg=0us tok=0 ...",
#     cache_hits: 0, cache_misses: 0, cache_evictions: 0}
```

> **Breaking change (gem ≥ next release):** `Emb.stats` used to return the raw
> RESP array (`["uptime_secs", 3, "total_requests", 0, ...]`). It now returns the
> Hash above. Callers using `stats.each_slice(2)` or array indexing must migrate.

### `Emb.server_info` — sectioned INFO, parsed

The Redis-style `INFO` reply is parsed into a nested Hash. **No arguments = all
sections**; pass section names to filter (`:server`, `:cache`, `:keyspace`, `:stats`, `:clients`):

```ruby
Emb.server_info
# => {Server: {redis_version: "0.2.4", emb_version: "0.2.4", uptime_secs: "7", ...},
#     Cache: {cache_hits: 0, cache_misses: 0, cache_hit_rate: "0.0%", ...},
#     Keyspace: {db0: "model=minilm,keys=0,hits=0,misses=0,hit_rate=0.0%"}, ...}

Emb.server_info(:server, :cache)   # only those two sections
```

### `Emb.config` — hot config read & change

`Emb.config` is a Hash-like live view of the server's runtime configuration
(backed by `CONFIG GET` / `CONFIG SET`). **Note:** the former `Emb.config` alias
for `Emb.setup` is gone — use `Emb.setup` to configure the client.

```ruby
Emb.config.to_h                          # all parameters
# => {"cache" => "auto", "cache_file" => "", "cache_save" => "", "listen" => ":6379",
#     "password" => "", "models" => "minilm,bge", "tls_cert" => "", "tls_key" => ""}

Emb.config['cache']                      # exact key => String value
# => "auto"
Emb.config['cache*']                     # glob => Hash of matching parameters
Emb.config['listen']                     # unknown key => nil

Emb.config['cache'] = '100MB'            # live change; returns "OK"
Emb.config['cache_file'] = '/var/lib/emb/cache.rdb'
```

Values are Strings (config is text, not metrics) and round-trip into writers
unchanged. `cache` resizes immediately, `cache_file`/`cache_save` apply at the
next snapshot save, `password` affects new connections. Errors surface as
exceptions (`RedisClient::CommandError`): read-only parameters (`listen`, `tls_*`,
`models`), invalid values, and `NOAUTH` on password-protected servers are not
swallowed.

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
`EMB.MULTI` on first use. Batching is **on by default**; the proxy API stays
eager only when you opt out with `batch: false`. `Emb.batch` works regardless
of the option, and `Emb.multi` remains the explicit, eager, deterministic
batching API.

### Clearing the cache per request

The per-thread batch scope holds cached embeddings for the life of the thread.
In a Rails application the bundled Railtie mounts `Emb::Middleware`
automatically, so the scope is cleared at the end of every request with no
configuration:

```ruby
# nothing to do — Emb::Railtie inserts Emb::Middleware for you
```

Opt out if you want to manage the stack yourself:

```ruby
# config/application.rb
config.emb.middleware = false
```

If your Gemfile loads `emb` before Rails is required (non-standard boot order), the
guarded railtie require in `emb.rb` is skipped — add `require "emb/railtie"` in
`config/application.rb` right after `require "rails/all"` (or in an initializer).

The middleware is also safe to mount manually in any Rack app (the Railtie
skips insertion when it is already present):

```ruby
use Emb::Middleware
```

The scope is cleared even when the app raises, and a fresh scope starts
automatically with the next request. Loaders created but never used within a
request are dropped — the create-then-consume contract applies per request.

### Job-scoped cache clearing

The Railtie also registers `Emb::JobMiddleware` — the same per-scope clearing
for background work — for every job framework present. Each job execution
starts with a fresh batch scope: cached embeddings and loaders created but
never used are dropped at the end of the job (even when it raises), so worker
threads never leak scope state between jobs.

- **ActiveJob** (SolidQueue, Sidekiq, Shoryuken, async, test adapters): an
  `around_perform` callback on `ActiveJob::Base` registered by the Railtie.
- **Plain Sidekiq workers** (non-ActiveJob): a server middleware added by the
  Railtie.
- **Plain Shoryuken workers** (non-ActiveJob): a server middleware added by the
  Railtie.

```ruby
# opt out of all job-scope protection
config.emb.job_middleware = false
```

In a non-Rails process, register the middleware manually:

```ruby
# sidekiq.rb / an initializer
Sidekiq.configure_server do |config|
  config.server_middleware { |chain| chain.add Emb::JobMiddleware }
end

# Shoryuken
Shoryuken.configure_server do |config|
  config.server_middleware { |chain| chain.add Emb::JobMiddleware }
end
```

The Railtie also registers `Emb::JobMiddleware` into the `Sidekiq::Testing` middleware
chain when `Sidekiq::Testing` is loaded *before the app finishes booting* (e.g. a dev
`INLINE_SIDEKIQ` initializer). For RSpec test modes, `sidekiq/testing` typically loads
after the app boots, so require it before your environment to get per-job clearing in
fake/inline tests — e.g. `gem "sidekiq", require: ["sidekiq", "sidekiq/testing"]`.

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
