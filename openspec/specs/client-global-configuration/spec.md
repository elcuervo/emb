# client-global-configuration Specification

## Purpose
Specifies a global configuration layer for the `emb` Ruby gem so every client (default,
`Emb.setup`, and `Emb.new`) picks up evidence-based out-of-the-box defaults — and
operators can override them once via `Emb.configure` — instead of repeating options at
every call site. `EMB_URL` remains the only environment variable.

## Requirements

### Requirement: Global configuration object

The gem SHALL expose a module-level configuration (`Emb.configure { |c| ... }` to set values and
`Emb.configuration` to read them) that feeds every client created afterwards
(`Emb.setup`, `Emb.new`, and the lazily-created default client). Explicit per-call options
SHALL take precedence over global configuration, which SHALL take precedence over built-in
defaults. The configuration surface SHALL include the `lazy` mode (`false`, `:multi`, or
`:batch`) in place of the removed `batch` boolean, and `url` SHALL accept a String or an
Array of Strings.

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

### Requirement: Opt-in batch retries via reconnect_attempts

The gem SHALL keep `reconnect_attempts: 0` as the default (a failing batch fails closed after a single attempt), while allowing operators to opt into bounded retries by setting `reconnect_attempts` to a value greater than zero: a transiently failing `EMB.MULTI` in a batch is then re-sent up to that many additional times before the batch fails closed with `Emb::ServerError`. Operation errors SHALL never be retried, regardless of the setting. The retry budget SHALL be capped by `reconnect_attempts` and every exhausted batch SHALL terminate in a visible `Emb::ServerError`, never an endless silent re-send.

#### Scenario: Default fails closed after one attempt

- **WHEN** an `EMB.MULTI` fails transiently under the default configuration (`reconnect_attempts: 0`)
- **THEN** exactly one command SHALL have been sent for the batch
- **AND** the batch SHALL fail closed by raising `Emb::ServerError`

#### Scenario: Setting reconnect_attempts engages retries

- **WHEN** `Emb.configure { |c| c.reconnect_attempts = 2 }` is set
- **AND** an `EMB.MULTI` fails transiently
- **THEN** up to 3 commands SHALL be sent for the batch (the initial attempt plus 2 retries)
- **AND** if all three fail, the batch SHALL fail closed by raising `Emb::ServerError`

#### Scenario: Operation errors never retry

- **WHEN** `Emb.configure { |c| c.reconnect_attempts = 2 }` is set
- **AND** the server returns an error reply for the batch
- **THEN** exactly one command SHALL have been sent
- **AND** the batch SHALL fail closed by raising `Emb::ServerError`

#### Scenario: Defaults documented

- **WHEN** a contributor reads the gem README
- **THEN** they SHALL find the out-of-the-box config (batching on, pool 5, driver, 10s timeouts, `reconnect_attempts: 0`), the failure taxonomy (transient errors retry when configured, operation errors never), and how to override globally or per call
