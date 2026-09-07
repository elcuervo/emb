## MODIFIED Requirements

### Requirement: Proxy-based API

The gem SHALL expose a module-level `Emb[name]` syntax that returns a memoized proxy for each model name. Instance clients SHALL expose the same `client[name]` syntax. The gem SHALL expose `Emb::VERSION` that resolves correctly regardless of install path.

Proxy embed-call behavior SHALL be governed by the `lazy` mode (per the `ruby-batch-loading` capability): with the default `lazy: false`, proxy embed calls SHALL immediately send `EMB` to the server and return the embedding; with `lazy: :multi` or `lazy: :batch`, they SHALL return a lazy embedding that materializes on first use. There SHALL be no explicit `Emb.batch` / `client.batch` proxy.

#### Scenario: Version resolves from loaded spec

- **WHEN** `require "emb"` is called
- **THEN** `Emb::VERSION` SHALL be a semver string matching the gem's version
- **THEN** the version SHALL NOT depend on any file relative to the gem's install directory

#### Scenario: Single embed (module level)

- **WHEN** `Emb[:minilm]["hello world"]` is called with the default `lazy: false` configuration
- **THEN** it SHALL send `EMB minilm "hello world"` at call time and return an Array of Float

#### Scenario: Single embed (instance)

- **WHEN** `client = Emb.new; client[:minilm]["hello world"]` is called with the default `lazy: false` configuration
- **THEN** it SHALL send `EMB minilm "hello world"` immediately and return an Array of Float

#### Scenario: Multi-text embed

- **WHEN** `Emb[:minilm]["hello", "world"]` is called with the default `lazy: false` configuration
- **THEN** it SHALL send `EMB minilm "hello" "world"` to the server
- **THEN** it SHALL return an Array of Array of Float

#### Scenario: Proxy is memoized

- **WHEN** `Emb[:minilm]` is called twice
- **THEN** the same `Emb::Proxy` object SHALL be returned both times

#### Scenario: Single embed in batch mode

- **WHEN** `Emb.setup(lazy: :batch)` has configured the default client
- **AND** `vec = Emb[:minilm]["hello world"]` is called
- **THEN** no command SHALL be sent to the server at call time
- **AND** when `vec` is used (e.g. `vec.sum`), it SHALL return an Array of Float
- **AND** the value SHALL equal the embedding the eager API would return for `minilm`/`"hello world"`

#### Scenario: Multi-text embed in batch mode

- **WHEN** `Emb.setup(lazy: :batch)` has configured the default client
- **AND** `vecs = Emb[:minilm]["hello", "world"]` is called
- **AND** `vecs` is used
- **THEN** it SHALL return an Array of Array of Float

#### Scenario: Eager path unaffected by explicit batch API

- **WHEN** a client has the default `lazy: false` configuration
- **THEN** the default proxy path `Emb[:minilm]["hello"]` SHALL remain eager
- **AND** the explicit `Emb.batch` / `client.batch` API SHALL NOT exist (invoking it raises `NoMethodError`), so it SHALL have no effect on the eager path

### Requirement: Multi-model batch

The gem SHALL support batch multi-model embedding via a block syntax, on both module level and instance level. The composed `EMB.MULTI` SHALL be split into commands of at most the configured `batch_size` pairs (default 512), preserving result ordering and per-pair nil behavior across chunks. The block syntax SHALL execute eagerly in every `lazy` mode: composing and running a block SHALL send `EMB.MULTI` at block end regardless of the configured mode.

#### Scenario: Multi-embed block (module level)

- **WHEN** `Emb.multi { |m| m[:minilm]["hello"]; m[:bge]["world"] }` is called with a default batch_size of 512
- **THEN** it SHALL send `EMB.MULTI minilm "hello" bge "world"` in a single command (2 pairs ≤ 512)
- **THEN** each result SHALL be unpacked from float32 binary to an Array of Float

#### Scenario: Multi-embed block (instance)

- **WHEN** `client.multi { |m| m[:minilm]["hello"]; m[:bge]["world"] }` is called
- **THEN** it SHALL return unpacked float arrays, same as the module-level call

#### Scenario: Oversized block chunks and reassembles

- **GIVEN** `batch_size: 100`
- **WHEN** `Emb.multi` composes 250 pairs
- **THEN** the pairs SHALL be sent as three commands of 100, 100, and 50 pairs
- **AND** results SHALL be concatenated in composition order

#### Scenario: batch_size applies globally and per client

- **WHEN** `Emb.configure { |c| c.batch_size = 64 }` is set
- **THEN** default and `Emb.new` clients SHALL use 64-pair chunks
- **AND** `Emb.new(batch_size: 32)` SHALL override with 32

#### Scenario: Multi stays eager under deferred modes

- **WHEN** a client is configured `lazy: :batch`
- **AND** `Emb.multi { |m| m[:minilm]["hello"] }` is called
- **THEN** `EMB.MULTI minilm "hello"` SHALL be sent when the block ends (not deferred)
- **AND** the return value SHALL be the processed result, not a lazy value