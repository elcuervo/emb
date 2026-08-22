## MODIFIED Requirements

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