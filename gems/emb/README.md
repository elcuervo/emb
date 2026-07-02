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

### Connection pool

```ruby
Emb.setup(url: "redis://localhost:6379", pool: 10)
```

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
all available options. Only `pool` is handled by the gem — everything else passes
through to `RedisClient.new`.

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
