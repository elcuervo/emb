## MODIFIED Requirements

### Requirement: Evidence-based defaults

The gem SHALL ship defaults selected from the benchmark results in `BENCHMARK.md`: eager execution by default (`lazy: false`), `pool: 5`, pure-Ruby RESP driver, `protocol: 2`, explicit `read_timeout` and `write_timeout` of 10 seconds, and `reconnect_attempts: 0`. These SHALL be overridable via `Emb.configure`. A `nil` timeout or a reconnect greater than zero SHALL NOT be a silent default, because an automatic command re-send duplicates non-idempotent EMB.MULTI work. The `protocol` option SHALL accept only `2`: the gem speaks RESP2, and it does not decode the RESP3 maps the server emits for `EMB.MODELS`, `EMB.STATS`, `EMB.INFO`, and `CONFIG GET`. Setting any other value SHALL raise `ArgumentError` at configuration time (`Emb.configure`) or client construction time (`Emb.new`), so a client can never be created in a mode whose introspection commands silently return wrong results.

#### Scenario: Default execution is eager

- **WHEN** `Emb.new` is called with no `lazy` option
- **THEN** proxy embed calls SHALL send `EMB` immediately (one round trip per call)

#### Scenario: Default timeouts are non-nil and forwarded

- **WHEN** `Emb.new` is called with no explicit timeout options
- **THEN** the underlying RedisClient SHALL be created with `read_timeout: 10` and `write_timeout: 10` (not nil, which falls back to redis-client's 1.0s default)

#### Scenario: No automatic command re-send by default

- **WHEN** an `EMB.MULTI` times out under the default configuration
- **THEN** exactly one command SHALL have been sent for the batch (no automatic re-sends), matching `reconnect_attempts: 0`

#### Scenario: Defaults documented

- **WHEN** a contributor reads the gem README
- **THEN** they SHALL find the out-of-the-box config (eager `lazy: false`, pool 5, driver, 10s timeouts, reconnect 0, RESP2-only protocol), the `lazy` mode options (`:multi` and `:batch`), multi-instance `url` arrays, and the benchmark rationale

#### Scenario: Unsupported protocol is rejected

- **WHEN** `Emb.configure { |c| c.protocol = 3 }` is called
- **THEN** the call SHALL raise `ArgumentError`
- **WHEN** `Emb.new(protocol: 3)` is called
- **THEN** the call SHALL raise `ArgumentError` naming RESP2 as the only supported protocol
