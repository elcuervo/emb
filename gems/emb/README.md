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

Configure the connection pool (defaults shown):

```ruby
require "emb"

Emb.setup(host: "localhost", port: 6379, pool: 5)
```

`Emb.config` is an alias for `Emb.setup`.

## Usage

### Single text

```ruby
result = Emb[:minilm]["hello world"]
# => [0.0123, -0.0456, 0.0789, ...]  (384 floats)
```

### Multiple texts

```ruby
results = Emb[:minilm]["hello", "world"]
# => [[0.0123, ...], [-0.0456, ...]]
```

### Multi-model queries

Send texts to different models in one round trip:

```ruby
Emb.multi do |m|
  m[:minilm]["hello"]
  m[:bge]["world"]
end
# => EMB.MULTI minilm "hello" bge "world"
```

### Commands

```ruby
Emb.models   # => [{name: "minilm", dim: 384, status: "ready"}, ...]
Emb.info(:minilm)  # => {dim: 384, workers: 10, requests: 42, ...}
Emb.stats    # => server statistics hash
Emb.help     # => command reference string
Emb.ping     # => "PONG"
```

## Testing end to end

Start the emb server, then run the test suite:

```bash
# From the repo root:
./bin/emb -config test-two-models.yaml &

# From gems/emb/:
bundle exec rake
```

Tests cover all commands: `EMB`, `EMB.MODELS`, `EMB.INFO`, `EMB.HELP`, `PING`, and `EMB.MULTI`.
