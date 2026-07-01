# gems-emb-ci

## Purpose

Specifies the structural and CI requirements for the `emb` Ruby gem located at `gems/emb/`, including its directory layout, test setup, documentation, and standalone CI workflow.

## Requirements

### Requirement: Gem directory structure

The `emb` Ruby gem SHALL reside at `gems/emb/` relative to the repo root.

#### Scenario: Move preserves all files

- **WHEN** the gem is moved from `gem/` to `gems/emb/`
- **THEN** all library files, specs, dependencies, and configuration SHALL be present at the new location

### Requirement: Rake test task

The gem SHALL provide a `Rakefile` where `rake` runs the full test suite.

#### Scenario: rake runs specs

- **WHEN** `rake` is executed in `gems/emb/`
- **THEN** all RSpec examples in `spec/` SHALL run
- **THEN** the exit code SHALL be 0 on success

### Requirement: E2E test documentation

The gem SHALL include a `README.md` documenting how to run end-to-end tests.

#### Scenario: README contains test instructions

- **WHEN** a developer reads `gems/emb/README.md`
- **THEN** it SHALL list prerequisites (emb binary, Ruby, bundler)
- **THEN** it SHALL describe how to start the emb server
- **THEN** it SHALL describe how to run `rake` for the test suite
- **THEN** it SHALL show basic usage examples

### Requirement: Standalone CI

The gem SHALL have its own CI workflow that builds `emb` and runs the gem's test suite.

#### Scenario: CI runs gem tests

- **WHEN** the CI workflow at `gems/emb/.github/workflows/test.yml` runs
- **THEN** it SHALL install Go, ONNX Runtime, and libtokenizers
- **THEN** it SHALL build the `emb` binary
- **THEN** it SHALL start the `emb` server with a test config
- **THEN** it SHALL run `cd gems/emb && bundle exec rake`
- **THEN** all tests SHALL pass
