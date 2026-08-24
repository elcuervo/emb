# emb-ruby-client

## Purpose

Specifies the Ruby client gem (`emb`) that provides a Redis-based interface to the emb server
with proxy-based API, command wrappers, multi-model batching, instance-based clients, URL configuration,
and connection pooling.

## Requirements

### Requirement: Instance-based client

The gem SHALL expose `Emb.new` that returns a standalone client instance with independent
connection pool and proxy registry.

#### Scenario: Create client with URL

- **WHEN** `client = Emb.new(url: "redis://10.0.0.1:6380")` is called
- **THEN** `client` SHALL be an `Emb::Client` instance
- **THEN** `client.ping` SHALL return `"PONG"` by connecting to `10.0.0.1:6380`

#### Scenario: Create client with host/port

- **WHEN** `client = Emb.new(host: "localhost", port: 6380)` is called
- **THEN** `client` SHALL connect to `localhost:6380`

#### Scenario: Create client with both URL and host/port

- **WHEN** `client = Emb.new(url: "redis://10.0.0.1:6380", host: "localhost")`
- **THEN** the URL SHALL take precedence over host/port

#### Scenario: EMB_URL env var default

- **WHEN** `ENV["EMB_URL"]` is set to `"redis://10.0.0.1:6380"`
- **AND** `client = Emb.new` is called without arguments
- **THEN** `client` SHALL connect to `10.0.0.1:6380`

#### Scenario: Default URL fallback

- **WHEN** `client = Emb.new` is called without arguments and no `EMB_URL` env var
- **THEN** `client` SHALL connect to `redis://localhost:6379`

#### Scenario: Independent clients

- **WHEN** `c1 = Emb.new(url: "redis://10.0.0.1:6380")` and `c2 = Emb.new(url: "redis://10.0.0.2:6380")`
- **THEN** `c1` and `c2` SHALL have separate connection pools
- **THEN** `c1[:minilm]` and `c2[:minilm]` SHALL return separate proxy instances

#### Scenario: Pool size configurable

- **WHEN** `client = Emb.new(url: "redis://localhost:6379", pool: 10)` is called
- **THEN** the connection pool SHALL have size 10

### Requirement: Proxy-based API

The gem SHALL expose a module-level `Emb[name]` syntax that returns a memoized proxy for each model name.
Instance clients SHALL expose the same `client[name]` syntax.
The gem SHALL expose `Emb::VERSION` that resolves correctly regardless of install path.
When the client is configured with `batch: false` (the default), proxy embed calls SHALL immediately send `EMB` to the server and return the embedding.
When the client is configured with `batch: true`, proxy embed calls SHALL return a lazy batched embedding (per the `ruby-batch-loading` capability) instead of sending a command immediately.

#### Scenario: Version resolves from loaded spec

- **WHEN** `require "emb"` is called
- **THEN** `Emb::VERSION` SHALL be a semver string matching the gem's version
- **THEN** the version SHALL NOT depend on any file relative to the gem's install directory

#### Scenario: Single embed (module level)

- **WHEN** `Emb[:minilm]["hello world"]` is called with the default `batch: false` configuration
- **THEN** it SHALL send `EMB minilm "hello world"` to the server
- **THEN** it SHALL return an Array of Float

#### Scenario: Single embed (instance)

- **WHEN** `client = Emb.new; client[:minilm]["hello world"]` is called with the default `batch: false` configuration
- **THEN** it SHALL return an Array of Float

#### Scenario: Multi-text embed

- **WHEN** `Emb[:minilm]["hello", "world"]` is called with the default `batch: false` configuration
- **THEN** it SHALL send `EMB minilm "hello" "world"` to the server
- **THEN** it SHALL return an Array of Array of Float

#### Scenario: Proxy is memoized

- **WHEN** `Emb[:minilm]` is called twice
- **THEN** the same `Emb::Proxy` object SHALL be returned both times

#### Scenario: Single embed in batch mode

- **WHEN** `Emb.setup(batch: true)` has configured the default client
- **AND** `vec = Emb[:minilm]["hello world"]` is called
- **THEN** no command SHALL be sent to the server at call time
- **AND** when `vec` is used (e.g. `vec.sum`), it SHALL return an Array of Float
- **AND** the value SHALL equal the embedding the eager API would return for `minilm`/`"hello world"`

#### Scenario: Multi-text embed in batch mode

- **WHEN** `Emb.setup(batch: true)` has configured the default client
- **AND** `vecs = Emb[:minilm]["hello", "world"]` is called
- **AND** `vecs` is used
- **THEN** it SHALL return an Array of Array of Float

#### Scenario: Eager path unaffected by explicit batch API

- **WHEN** `Emb.batch[:minilm]["hello"]` is used while the client has `batch: false`
- **THEN** the explicit batch API SHALL still return a lazy batched embedding
- **AND** the default proxy path `Emb[:minilm]["hello"]` SHALL remain eager

### Requirement: Command wrappers

The gem SHALL expose module-level methods for all server commands, delegating to the default client.
Instance clients SHALL expose the same methods.

#### Scenario: List models

- **WHEN** `Emb.models` or `client.models` is called
- **THEN** it SHALL send `EMB.MODELS` and return an array of `{name:, dim:, status:}` hashes

#### Scenario: Model info

- **WHEN** `Emb.info(:minilm)` or `client.info(:minilm)` is called
- **THEN** it SHALL send `EMB.INFO minilm` and return a hash of key-value pairs

#### Scenario: Server stats

- **WHEN** `Emb.stats` or `client.stats` is called
- **THEN** it SHALL send `EMB.STATS` and return the parsed response

#### Scenario: Help text

- **WHEN** `Emb.help` or `client.help` is called
- **THEN** it SHALL send `EMB.HELP` and return the response string

#### Scenario: Ping

- **WHEN** `Emb.ping` or `client.ping` is called
- **THEN** it SHALL send `PING` and return `"PONG"`

### Requirement: Multi-model batch

The gem SHALL support batch multi-model embedding via a block syntax, on both module level and instance level.

#### Scenario: Multi-embed block (module level)

- **WHEN** `Emb.multi { |m| m[:minilm]["hello"]; m[:bge]["world"] }` is called
- **THEN** it SHALL send `EMB.MULTI minilm "hello" bge "world"` in a single command
- **THEN** each result SHALL be unpacked from float32 binary to an Array of Float

#### Scenario: Multi-embed block (instance)

- **WHEN** `client.multi { |m| m[:minilm]["hello"]; m[:bge]["world"] }` is called
- **THEN** it SHALL return unpacked float arrays, same as the module-level call

### Requirement: Connection pooling

The gem SHALL use `ConnectionPool` wrapping `RedisClient` to reuse connections.
The default pool size SHALL be the benchmark-derived value selected by the pool-size
sweep and published in `BENCHMARK.md`, and SHALL be documented in the gem README.

#### Scenario: Default pool size

- **WHEN** `Emb.setup` is called without a pool size
- **THEN** the pool SHALL default to the benchmark-derived size published in `BENCHMARK.md`

#### Scenario: Pool default is evidence-based

- **WHEN** a contributor reads `BENCHMARK.md`
- **THEN** it SHALL document the pool-size sweep results (pool sizes vs p50/p99 at the
  benchmarked concurrency) and the selected default

#### Scenario: Custom config via setup

- **WHEN** `Emb.setup(host: "10.0.0.1", port: 6380, pool: 10)` is called
- **THEN** connections SHALL go to `10.0.0.1:6380` with a pool of 10
- **THEN** the global `Emb.ping`, `Emb[:model]`, etc. SHALL use this configuration

#### Scenario: Lazy default client

- **WHEN** `Emb.ping` is called without prior `Emb.setup`
- **THEN** a default client SHALL be created automatically with `EMB_URL` or `redis://localhost:6379`

### Requirement: Driver selection

The gem SHALL document RESP driver selection and its performance impact in the README.
The pure-Ruby driver SHALL remain the default unless the benchmark harness shows the
`hiredis` driver is clearly better (meeting the documented improvement threshold), in
which case the gem SHALL switch the default and note the native dependency.

#### Scenario: Driver is configurable

- **WHEN** a client is created with `Emb.setup(driver: :hiredis)` or `Emb.new(driver: :hiredis)`
- **THEN** the option SHALL pass through to `RedisClient` and take effect

#### Scenario: Driver tradeoffs documented

- **WHEN** a contributor reads the README
- **THEN** they SHALL find the benchmark numbers comparing the pure-Ruby and `hiredis`
  drivers and the reason for the current default
