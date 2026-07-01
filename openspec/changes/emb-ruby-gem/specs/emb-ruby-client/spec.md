## ADDED Requirements

### Requirement: Proxy-based API

The gem SHALL expose a module-level `Emb[name]` syntax that returns a memoized proxy for each model name.

#### Scenario: Single embed

- **WHEN** `Emb[:minilm]["hello world"]` is called
- **THEN** it SHALL send `EMB minilm "hello world"` to the server
- **THEN** it SHALL return the raw binary response

#### Scenario: Multi-text embed

- **WHEN** `Emb[:minilm]["hello", "world"]` is called
- **THEN** it SHALL send `EMB minilm "hello" "world"` to the server

#### Scenario: Proxy is memoized

- **WHEN** `Emb[:minilm]` is called twice
- **THEN** the same `Emb::Proxy` object SHALL be returned both times

### Requirement: Command wrappers

The gem SHALL expose module-level methods for all server commands.

#### Scenario: List models

- **WHEN** `Emb.models` is called
- **THEN** it SHALL send `EMB.MODELS` and return an array of `{name:, dim:, status:}` hashes

#### Scenario: Model info

- **WHEN** `Emb.info(:minilm)` is called
- **THEN** it SHALL send `EMB.INFO minilm` and return a hash of key-value pairs

#### Scenario: Server stats

- **WHEN** `Emb.stats` is called
- **THEN** it SHALL send `EMB.STATS` and return the parsed response

#### Scenario: Help text

- **WHEN** `Emb.help` is called
- **THEN** it SHALL send `EMB.HELP` and return the response string

#### Scenario: Ping

- **WHEN** `Emb.ping` is called
- **THEN** it SHALL send `PING` and return `"PONG"`

### Requirement: Multi-model batch

The gem SHALL support batch multi-model embedding via a block syntax.

#### Scenario: Multi-embed block

- **WHEN** `Emb.multi { |m| m[:minilm]["hello"]; m[:bge]["world"] }` is called
- **THEN** it SHALL send `EMB.MULTI minilm "hello" bge "world"` in a single command

### Requirement: Connection pooling

The gem SHALL use `ConnectionPool` wrapping `RedisClient` to reuse connections.

#### Scenario: Default pool size

- **WHEN** `Emb.setup` is called without a pool size
- **THEN** the pool SHALL default to 5 connections

#### Scenario: Custom config

- **WHEN** `Emb.setup(host: "10.0.0.1", port: 6380, pool: 10)` is called
- **THEN** connections SHALL go to `10.0.0.1:6380` with a pool of 10

