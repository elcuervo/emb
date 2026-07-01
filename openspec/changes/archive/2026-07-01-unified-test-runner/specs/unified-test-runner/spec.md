## ADDED Requirements

### Requirement: Single command runs all tests

The SHALL be a `just all` recipe that runs the complete test suite for the project.

#### Scenario: full test passes

- **WHEN** `just all` is run
- **THEN** it SHALL run `just test` (Go server tests)
- **THEN** it SHALL build the `emb` binary via `just build`
- **THEN** it SHALL start the `emb` server with `test-two-models.yaml`
- **THEN** it SHALL run the `gems/emb` RSpec suite via `bundle exec rake`
- **THEN** it SHALL build the `gems/emb-server` gem to validate it
- **THEN** it SHALL stop the `emb` server
- **THEN** the exit code SHALL be 0

#### Scenario: fail fast on Go test failure

- **WHEN** `just test` fails
- **THEN** `just all` SHALL stop immediately without starting the server or running gem tests

#### Scenario: fail fast on gem test failure

- **WHEN** the RSpec suite fails
- **THEN** `just all` SHALL stop without building the `emb-server` gem
