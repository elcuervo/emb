# rubocop-dev-tooling

## Purpose

Specifies dev tooling configuration for Ruby gems — RuboCop linting rules, interactive console,
and build workflow. This is a tooling-only change with no runtime behavior impact.

## Requirements

### Requirement: RuboCop linting

Both gems SHALL have RuboCop configured and pass with zero warnings.

#### Scenario: RuboCop passes for emb gem

- **WHEN** `bundle exec rubocop` is run in `gems/emb/`
- **THEN** it SHALL exit with status 0 and no offenses

#### Scenario: RuboCop passes for emb-server gem

- **WHEN** `bundle exec rubocop` is run in `gems/emb-server/`
- **THEN** it SHALL exit with status 0 and no offenses

### Requirement: Interactive console

Both gems SHALL provide a Rake task to start an IRB session with the gem loaded.

#### Scenario: emb console

- **WHEN** `bundle exec rake console` is run in `gems/emb/`
- **THEN** it SHALL start IRB with the `emb` library loaded

#### Scenario: emb-server console

- **WHEN** `bundle exec rake console` is run in `gems/emb-server/`
- **THEN** it SHALL start IRB with the `emb-server` library loaded
