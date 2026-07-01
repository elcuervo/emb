# emb — Ruby client

Thin Ruby wrapper for [emb](https://github.com/elcuervo/emb), a Redis-compatible embedding server.

## Usage

```ruby
require "emb"

Emb.setup(host: "localhost", port: 6379, pool: 5)

Emb[:minilm]["hello world"]
# → raw float32 bytes

Emb.models
# → [{name: "minilm", dim: 384, status: "ready"}, ...]

Emb.info(:minilm)
# → {dim: 384, workers: 10, ...}

Emb.multi do |m|
  m[:minilm]["hello"]
  m[:bge]["world"]
end
# → EMB.MULTI minilm "hello" bge "world"
```

## Testing end to end

### Prerequisites

- An `emb` binary built from the repo root (`just build` or `go build`)
- Ruby 3.3+ with bundler

### Running tests

```bash
# From the repo root, start the emb server:
./bin/emb -config test-two-models.yaml &

# From gems/emb/, run the test suite:
bundle exec rake
```

This starts the full test suite against a real running `emb` server. Tests cover all commands: `EMB`, `EMB.MODELS`, `EMB.INFO`, `EMB.HELP`, `PING`, and `EMB.MULTI`.
