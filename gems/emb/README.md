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
  c.lazy = :multi   # defer and coalesce embed calls app-wide
end

Emb.configuration   # => the shared Emb::Configuration
Emb::Client.new(pool: 20)  # per-call still wins
```

`EMB_URL` remains the only environment variable (connection URL fallback, as before);
`Emb.configure { |c| c.url = ... }` or an explicit `url:` override it.

### Out-of-the-box defaults

The shipped defaults are benchmark-derived (see `BENCHMARK.md`) with secure-by-default
network behavior: **eager execution by default** (`lazy: false` — every `Emb[:model][t]`
sends one `EMB` round trip), pool `5`, pure-Ruby RESP driver, `protocol: 2`,
`read_timeout`/`write_timeout` both **10s**, and `reconnect_attempts: 0`. Opt into
coalescing with `lazy: :multi` or concurrent fan-out with `lazy: :batch` — globally via
`Emb.configure { |c| c.lazy = :batch }` or per client with `Emb.new(lazy: :batch)`.

**Why the timeout and reconnect defaults matter:** batched `EMB.MULTI` (up to 512 pairs)
can take over a second of inference on a shared CPU, and redis-client's silent default is
1.0s — a slower reply times out and (because `ReadTimeoutError` is a `ConnectionError`)
redis-client **re-sends the whole command** `reconnect_attempts + 1` times, duplicating
server inference. The gem therefore defaults to an explicit 10s timeout and **no
automatic re-send** (`reconnect_attempts: 0`); recovery is the caller's choice, and batch
failures fail closed (see [Lazy execution modes](#lazy-execution-modes)). Raise the
timeouts if you raise `batch_size`; opt into one automatic retry with
`Emb.configure { |c| c.reconnect_attempts = 1 }` if you accept the duplicate-work risk.

### Connection pool

```ruby
Emb.setup(url: "redis://localhost:6379", pool: 10)
```

The default pool size is **5**. The client keeps `pool` persistent connections per
emb **instance** and routes commands through them **in round-robin order** —
consecutive commands use different connections instead of reusing one hot connection.

`url` also accepts an **array of instances** (interchangeable replicas serving the same
model set); commands then round-robin across instances first, then across connections
within the selected instance:

```ruby
Emb.setup(url: ["redis://emb-a:6379", "redis://emb-b:6379", "redis://emb-c:6379"], pool: 5)
```

Each url gets its own pool of `pool` connections, so `pool: 5` with three urls maintains
15 connections total. If an instance refuses a connection **before a command is sent**
(connection refused/unreachable), the command is retried on the next instance — this is
why `reconnect_attempts` stays `0`: a retry after a *sent* command could duplicate
inference, but a never-sent command is safe to move elsewhere.

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
`ConnectionPool::TimeoutError`). Only the wait for a free pool connection is
unbounded — once a command runs, `RedisClient`'s `connect_timeout`,
`read_timeout`, and `write_timeout` still apply. For inference workloads this
lets commands ride out token-budget batching instead of erroring.

Locally (a single server) only the backend selection is identical whatever the
pool size — every command reaches the same server. The pool size still controls
how many persistent connections the client holds and how many commands run in
parallel. The pool is usually not the bottleneck for
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
all available options. Only `pool`, `lazy`, and `batch_size` are handled by the
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
> may change between calls (with `pool: 1` it never does) — each call is a
> sample. `Emb.config[key] = value` mutates one (arbitrary) instance. Use
> per-connection queries if you need a specific or aggregated view.

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
sections**; pass section names to filter (`:server`, `:cache`, `:keyspace`, `:stats`, `:memory`, `:cpu`, `:clients`):

```ruby
Emb.server_info
# => {Server: {redis_version: "0.2.4", emb_version: "0.2.4", uptime_secs: "7", ...},
#     Cache: {cache_hits: 0, cache_misses: 0, cache_hit_rate: "0.0%", ...},
#     Keyspace: {db0: "model=minilm,keys=0,hits=0,misses=0,hit_rate=0.0%"},
#     Memory: {used_memory_rss_bytes: 262438912, used_memory_heap_bytes: 154960,
#              goroutines: 4, total_system_memory_bytes: 25769803776},
#     CPU: {used_cpu_user_usec: 188900, used_cpu_sys_usec: 50462, gomaxprocs: 10}, ...}

Emb.server_info(:memory, :cpu)   # live process resources only
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

### Lazy execution modes

Embed-call behavior is governed by a single `lazy` mode — `false` (default, eager),
`:multi` (defer and coalesce into one `EMB` for a single model / one `EMB.MULTI` for mixed scopes, serial), or `:batch` (defer and execute
the coalesced chunk shares **concurrently**). The three are mutually exclusive.

| mode | `Emb[:model][text]` | execution |
|---|---|---|
| `false` (default) | immediate `EMB` round trip | serial, per call |
| `:multi` | deferred → coalesces into one `EMB` (single model) or `EMB.MULTI` (mixed) | serial, one command at a time |
| `:batch` | deferred → coalesces into `EMB`/`EMB.MULTI` chunks | **concurrent** — chunk shares run in parallel |

In `:batch` mode the shares fan out across the configured instances (one share per
instance when the share count allows) or across the instance's pool connections when a
single url is configured — a batch whose pieces take 60ms and 10ms completes in roughly
the slowest share (~60ms) instead of the sum (~70ms). Deferred work is powered by the
[batch-loader](https://github.com/exAspArk/batch-loader) gem:

```ruby
Emb.setup(lazy: :multi)  # or :batch — Emb[:minilm] now defers

users = User.all # some application objects

# Create loaders first...
l1 = Emb[:minilm]["hello"]
l2 = Emb[:minilm]["world"]
l3 = Emb[:bge]["bonjour"]

# ...then consume them. The first use sends ONE EMB (single-model scope)
# or ONE EMB.MULTI (mixed-model scope) for all three.
l1.sum  # => 12.345
l2.sum  # => -0.678
l3.sum  # => 3.141
```

Each lazy value materializes to the same shape as the eager API: a single text
yields an `Array<Float>`, multiple texts yield `Array<Array<Float>>`. For explicit
composition that always sends immediately regardless of mode, use `Emb.multi { }`
(see [Multi-model queries](#multi-model-queries)).

The batch executes in the current thread's scope; the Rack/ActiveJob middleware
clears that scope after every request/job (`Emb::BatchScope`), so deferred work
can never accumulate across requests (in eager mode there is nothing to clear).

**Fail-closed batches.** If an `EMB.MULTI` fails (timeout, connection error, or — in
`:batch` mode — a share failure after pre-send retries are exhausted), the
error is raised to the code that first used the batch, and every deferred item of
that batch is removed from the scope — retrying (or using other items of the
failed batch) **does not re-send the batch** and resolves to `[]` instead. This
prevents a slow server from turning one failed batch into endless duplicate work
(retries re-running the whole batch) or growth of the pending set across retries.
Pair-level failures the server reports as `null` (MGET semantics) are unaffected.

```ruby
vec   = Emb[:minilm]["hello"]            # use -> Array of Float
vecs  = Emb[:minilm]["hello", "world"]  # use -> Array of Array of Float
```

Embeddings are cached per thread, so reusing a lazy value (or creating an
identical pair again in the same scope) is free after the first use. A pair whose
embedding fails materializes as `nil`, matching `EMB.MULTI`'s per-pair null
behavior; siblings in the same batch still succeed.

#### The create-then-consume contract

Loaders only fire when a value is **used**. Create all loaders *first*, then
consume them, so they share one round trip:

```ruby
texts.each { |t| process(Emb[:minilm][t]) }   # wrong: one EMB per item
loaders = texts.map { |t| Emb[:minilm][t] }   # right: ONE EMB for all
loaders.each { |l| process(l) }
```

A loader that is created but never used **never embeds** (unless a sibling batch
fires first) and is silently dropped when the thread's scope ends. Duration and
scope: batching is per-thread — a multithreaded app issues one `EMB`/`EMB.MULTI` per
thread per flush.

### `lazy` configuration option

Setting `lazy: :multi` or `lazy: :batch` makes the standard proxy API defer, so
existing call sites batch automatically without restructuring:

```ruby
Emb.setup(url: "redis://localhost:6379", lazy: :multi)
# or
Emb.new(url: "redis://localhost:6379", lazy: :batch)
```

Under `lazy: :multi`, `Emb[:minilm]["hello"]` returns a lazy embedding that sends
`EMB` on first use (serial chunks; `EMB.MULTI` only for mixed-model scopes). Under `lazy: :batch`, the chunk shares
execute concurrently — with multiple `url`s they fan out across instances. The
default is eager (`lazy: false`). `Emb.multi` remains the explicit, eager,
deterministic composition API in every mode.

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

a = client.pools.first.with do |conn|
  conn.pipelined do |pipe|
    texts.each { |t| pipe.call("EMB", "minilm", t) }
  end
end
# a -> Array of Float32-binary replies; unpack each with r.unpack("e*")
```

`Client#pools` exposes the per-instance pools' `RedisClient` connections (the
first pool for a single-instance client). This measurably beats
plain per-call round trips; use it for bursts, and `Emb.multi` or a deferred `lazy`
mode when you want
server-side coalescing into one packed inference.

### Benchmark harness coverage

The client benchmark harness (`gems/emb/bench/bench.rb`, `just bench-ruby`)
reports one row per execution mechanism: `eager` (one `EMB` per call), `multi`
(coalesced `EMB`, or `EMB.MULTI` for mixed models), `batch` (concurrent `EMB` chunk shares),
`pipelined` (raw RESP pipelining), and `threaded` (eager across threads).
`just bench-ruby-multi` starts two partitioned emb instances and adds the
url-array rows `eager-2node` (round-robin distribution) and `batch-2node`
(concurrent fan-out across instances); the two-node rows are skipped unless a
second instance is reachable (`EMB_BENCH_PORT2`).

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
horizontal scale compose: each instance receives one `EMB` (single model) or `EMB.MULTI` (mixed) share per request.

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
