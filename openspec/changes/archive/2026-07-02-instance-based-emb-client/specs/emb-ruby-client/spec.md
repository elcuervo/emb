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

#### Scenario: Version resolves from loaded spec

- **WHEN** `require "emb"` is called
- **THEN** `Emb::VERSION` SHALL be a semver string matching the gem's version
- **THEN** the version SHALL NOT depend on any file relative to the gem's install directory

#### Scenario: Single embed (module level)

- **WHEN** `Emb[:minilm]["hello world"]` is called
- **THEN** it SHALL send `EMB minilm "hello world"` to the server
- **THEN** it SHALL return an Array of Float

#### Scenario: Single embed (instance)

- **WHEN** `client = Emb.new; client[:minilm]["hello world"]` is called
- **THEN** it SHALL return an Array of Float

#### Scenario: Multi-text embed

- **WHEN** `Emb[:minilm]["hello", "world"]` is called
- **THEN** it SHALL send `EMB minilm "hello" "world"` to the server
- **THEN** it SHALL return an Array of Array of Float

#### Scenario: Proxy is memoized

- **WHEN** `Emb[:minilm]` is called twice
- **THEN** the same `Emb::Proxy` object SHALL be returned both times

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

#### Scenario: Default pool size

- **WHEN** `Emb.setup` is called without a pool size
- **THEN** the pool SHALL default to 5 connections

#### Scenario: Custom config via setup

- **WHEN** `Emb.setup(host: "10.0.0.1", port: 6380, pool: 10)` is called
- **THEN** connections SHALL go to `10.0.0.1:6380` with a pool of 10
- **THEN** the global `Emb.ping`, `Emb[:model]`, etc. SHALL use this configuration

#### Scenario: Lazy default client

- **WHEN** `Emb.ping` is called without prior `Emb.setup`
- **THEN** a default client SHALL be created automatically with `EMB_URL` or `redis://localhost:6379`
