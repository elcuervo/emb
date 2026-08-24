## MODIFIED Requirements

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

## ADDED Requirements

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