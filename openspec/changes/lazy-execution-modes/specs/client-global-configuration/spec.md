## REMOVED Requirements

### Requirement: Evidence-based out-of-the-box defaults

**Reason**: The default execution model flips from lazy batching (`batch: true`) to eager single-command sends (`lazy: false`), so a scenario named "Default batching is on" and the requirement's phrasing can no longer describe the shipped defaults.

**Migration**: The replacement requirement "Evidence-based defaults" states the new out-of-the-box configuration: eager `lazy: false`, `pool: 5`, pure-Ruby RESP driver, `protocol: 2`, explicit 10s timeouts, `reconnect_attempts: 0`. Opt into `lazy: :multi` or `lazy: :batch` where deferred/parallel execution is wanted.

## MODIFIED Requirements

### Requirement: Global configuration object

The gem SHALL expose a module-level configuration (`Emb.configure { |c| ... }` to set values and `Emb.configuration` to read them) that feeds every client created afterwards (`Emb.setup`, `Emb.new`, and the lazily-created default client). Explicit per-call options SHALL take precedence over global configuration, which SHALL take precedence over built-in defaults. The configuration surface SHALL include the `lazy` mode (`false`, `:multi`, or `:batch`) in place of the removed `batch` boolean, and `url` SHALL accept a String or an Array of Strings.

#### Scenario: Configure once applies to all clients

- **WHEN** `Emb.configure { |c| c.lazy = :batch; c.pool = 8 }` is called
- **AND** then `Emb.new(url: "redis://localhost:6379")` is created
- **THEN** the new client SHALL have batch mode enabled (`lazy: :batch`) and pool size 8
- **AND** subsequent `Emb.new` clients SHALL inherit the same settings

#### Scenario: Per-call option wins over global config

- **WHEN** `Emb.configure { |c| c.pool = 5 }` is set
- **AND** `Emb.new(url: "redis://localhost:6379", pool: 20)` is created
- **THEN** the pool SHALL be 20

#### Scenario: EMB_URL remains the only env var

- **WHEN** `ENV["EMB_URL"]` is set and no per-call `url:` is given
- **THEN** the client SHALL connect using `EMB_URL` (as before)
- **AND** no other `EMB_*` environment variable SHALL affect the gem

## ADDED Requirements

### Requirement: Evidence-based defaults

The gem SHALL ship defaults selected from the benchmark results in `BENCHMARK.md`: eager execution by default (`lazy: false`), `pool: 5`, pure-Ruby RESP driver, `protocol: 2`, explicit `read_timeout` and `write_timeout` of 10 seconds, and `reconnect_attempts: 0`. These SHALL be overridable via `Emb.configure`. A `nil` timeout or a reconnect greater than zero SHALL NOT be a silent default, because an automatic command re-send duplicates non-idempotent EMB.MULTI work.

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
- **THEN** they SHALL find the out-of-the-box config (eager `lazy: false`, pool 5, driver, 10s timeouts, reconnect 0), the `lazy` mode options (`:multi` and `:batch`), multi-instance `url` arrays, and the benchmark rationale