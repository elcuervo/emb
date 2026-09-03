## MODIFIED Requirements

### Requirement: Evidence-based out-of-the-box defaults

The gem SHALL ship defaults selected from the benchmark results in `BENCHMARK.md`: lazy batching enabled by default (`batch: true`), `pool: 5`, pure-Ruby RESP driver, `protocol: 2`, explicit `read_timeout` and `write_timeout` of 10 seconds, and `reconnect_attempts: 0`. These SHALL be overridable via `Emb.configure`. A `nil` timeout or a reconnect greater than zero SHALL NOT be a silent default, because an automatic command re-send duplicates non-idempotent EMB.MULTI work.

#### Scenario: Default batching is on

- **WHEN** `Emb.new` is called with no batching option
- **THEN** proxy embed calls SHALL be lazy (batch on by default)

#### Scenario: Default timeouts are non-nil and forwarded

- **WHEN** `Emb.new` is called with no explicit timeout options
- **THEN** the underlying RedisClient SHALL be created with `read_timeout: 10` and `write_timeout: 10` (not nil, which falls back to redis-client's 1.0s default)

#### Scenario: No automatic command re-send by default

- **WHEN** an `EMB.MULTI` times out under the default configuration
- **THEN** exactly one command SHALL have been sent for the batch (no automatic re-sends), matching `reconnect_attempts: 0`

#### Scenario: Defaults documented

- **WHEN** a contributor reads the gem README
- **THEN** they SHALL find the out-of-the-box config (batching on, pool 5, driver, 10s timeouts, reconnect 0) and the benchmark rationale, plus how to override globally or per call