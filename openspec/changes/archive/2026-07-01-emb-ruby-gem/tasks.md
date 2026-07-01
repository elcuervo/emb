## 1. Scaffold Gem

- [x] 1.1 Create `gem/` directory with `gem/emb.gemspec`, `gem/Gemfile`, `gem/VERSION` (content: `0.1.0`)
- [x] 1.2 Create `gem/lib/emb.rb` entry point loading all submodules
- [x] 1.3 Create `gem/lib/emb/version.rb` reading VERSION file

## 2. Client and Connection Pool

- [x] 2.1 Create `gem/lib/emb/client.rb` with `ConnectionPool` + `RedisClient`, exposes `Emb.setup(host:, port:, pool:)` and internal `send_command` method

## 3. Proxy and Registry

- [x] 3.1 Create `gem/lib/emb/proxy.rb` with `Emb::Proxy.new(name)` memoized in module-level `@registry`
- [x] 3.2 Implement `Emb.[]` (registry lookup with memoization)
- [x] 3.3 Implement `Emb::Proxy#[](text, *texts)` → sends `EMB <name> <text> [text...]`
- [x] 3.4 Implement `Emb::Proxy#inspect` returning `#<Emb::Proxy minilm>`

## 4. Test suite against a running emb server

- [x] 4.1 Create `gem/spec/spec_helper.rb` that starts an `emb` server process (via `just build` + config with a test model), waits for it to be ready, and cleans up after
- [x] 4.2 Test `Emb[:model]["text"]` returns expected binary data and handles multi-text
- [x] 4.3 Test `Emb.models`, `Emb.info`, `Emb.stats`, `Emb.help`, `Emb.ping` return correct responses
- [x] 4.4 Test `Emb.multi { |m| m[:a]["t1"]; m[:b]["t2"] }` sends batch and returns array
- [x] 4.5 Test `Emb.setup` with custom host/port/pool
- [x] 4.6 Test proxy memoization (same `Emb[:x]` returns same object)

## 5. Top-level Commands

- [x] 5.1 Implement `Emb.models` → `EMB.MODELS`, parse RESP array to `[{name:, dim:, status:}]`
- [x] 5.2 Implement `Emb.info(name)` → `EMB.INFO <name>`, parse RESP array to Hash
- [x] 5.3 Implement `Emb.stats` → `EMB.STATS`, parse RESP bulk string to Hash
- [x] 5.4 Implement `Emb.help` → `EMB.HELP`, return raw string
- [x] 5.5 Implement `Emb.ping` → `PING`, return `"PONG"`

## 6. Multi-model Batch

- [x] 6.1 Create `gem/lib/emb/multi.rb` with `Emb::MultiProxy`
- [x] 6.2 `MultiProxy#[](name)` returns a queue-proxy that collects `(name, text)` pairs
- [x] 6.3 `Emb.multi { |m| ... }` yields `MultiProxy`, sends `EMB.MULTI` on block return

## 7. Flake.nix Update

- [x] 7.1 Add `ruby_3_4` and `bundler` to `flake.nix` devShell `buildInputs`
