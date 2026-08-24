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
defaults.

#### Scenario: Configure once applies to all clients

- **WHEN** `Emb.configure { |c| c.batch = false; c.pool = 8 }` is called
- **AND** then `Emb.new(url: "redis://localhost:6379")` is created
- **THEN** the new client SHALL have batching disabled and pool size 8
- **AND** subsequent `Emb.new` clients SHALL inherit the same settings

#### Scenario: Per-call option wins over global config

- **WHEN** `Emb.configure { |c| c.pool = 5 }` is set
- **AND** `Emb.new(url: "redis://localhost:6379", pool: 20)` is created
- **THEN** the pool SHALL be 20

#### Scenario: EMB_URL remains the only env var

- **WHEN** `ENV["EMB_URL"]` is set and no per-call `url:` is given
- **THEN** the client SHALL connect using `EMB_URL` (as before)
- **AND** no other `EMB_*` environment variable SHALL affect the gem

### Requirement: Evidence-based out-of-the-box defaults

The gem SHALL ship defaults selected from the benchmark results in `BENCHMARK.md`: lazy
batching enabled by default (`batch: true`), `pool: 5`, pure-Ruby RESP driver,
`protocol: 2`, and `reconnect_attempts: 3`. These SHALL be overridable via
`Emb.configure`.

#### Scenario: Default batching is on

- **WHEN** `Emb.new` is called with no batching option
- **THEN** proxy embed calls SHALL be lazy (batch on by default)

#### Scenario: Defaults documented

- **WHEN** a contributor reads the gem README
- **THEN** they SHALL find the out-of-the-box config (batching on, pool 5, driver) and the
  benchmark rationale, plus how to override globally or per call
