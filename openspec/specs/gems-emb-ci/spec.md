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

### Requirement: Real-Rails boot validation

The `emb` gem's test suite SHALL exercise the Railtie against a real Rails application rather than only against hand-rolled Rails fakes. Rails SHALL be a development-only dependency, and the suite SHALL boot a minimal application and assert middleware behavior against the application's built middleware stack after initialization.

#### Scenario: Real Rails app boots without raising

- **WHEN** the integration suite boots a minimal Rails application with the gem loaded
- **THEN** initialization SHALL succeed
- **THEN** the built middleware stack SHALL include `Emb::Middleware` exactly once

#### Scenario: Opt-out is asserted on the built stack

- **WHEN** the integration suite boots the application with `config.emb.middleware = false`
- **THEN** the built middleware stack SHALL NOT include `Emb::Middleware`

#### Scenario: Duplicate mount remains functionally safe

- **WHEN** the application mounts `Emb::Middleware` manually and the Railtie also inserts it
- **THEN** every request through the application SHALL leave the batch scope cleared

#### Scenario: Test doubles cannot exceed the real API

- **WHEN** the suite defines a stand-in for a Rails internal used by the Railtie
- **THEN** a validation SHALL fail if the stand-in exposes a method the real Rails class does not define

#### Scenario: Supported Rails versions are exercised in CI

- **WHEN** CI runs the gem test job
- **THEN** it SHALL run the integration suite for each supported Ruby and Rails version
- **THEN** the suite SHALL run as part of the gem's default `rake` task
