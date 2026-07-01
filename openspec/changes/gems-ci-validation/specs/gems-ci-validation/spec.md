## ADDED Requirements

### Requirement: Local gem validation

There SHALL be a `just validate-gems` recipe that builds, installs, and validates both gems without pushing to rubygems.org.

#### Scenario: emb gem validated locally

- **WHEN** `just validate-gems` runs
- **THEN** `gem build emb.gemspec` SHALL succeed in `gems/emb/`
- **THEN** `gem install --local emb-*.gem` SHALL succeed
- **THEN** `ruby -e "require 'emb'; puts Emb::VERSION"` SHALL print the version

#### Scenario: emb-server gem validated locally

- **WHEN** `just validate-gems` runs
- **THEN** a platform binary SHALL be copied into `gems/emb-server/lib/emb-server/`
- **THEN** `gem build emb-server.gemspec` SHALL succeed
- **THEN** `gem install --local emb-server-*.gem` SHALL succeed (requires onnxruntime gem installed)
- **THEN** `which emb` SHALL find the binary on PATH

### Requirement: act for local CI testing

The nix devShell SHALL include `act` for running GitHub Actions workflows locally.

#### Scenario: act runs gem CI

- **WHEN** `act run --workflows gems/emb/.github/workflows/test.yml` is run
- **THEN** the workflow SHALL execute locally (requires Docker running)

### Requirement: Gem CI trigger alignment

Both gem CI workflows SHALL trigger on tag pushes and support `workflow_dispatch`.

#### Scenario: Tag push triggers gem CIs

- **WHEN** a tag matching `v*` is pushed
- **THEN** `gems/emb/.github/workflows/test.yml` SHALL run
- **THEN** `gems/emb-server/.github/workflows/test.yml` SHALL run

### Requirement: emb-server standalone CI

The `gems/emb-server/` gem SHALL have its own CI workflow that validates the gem builds correctly.

#### Scenario: emb-server CI validates gem build

- **WHEN** the `gems/emb-server` CI runs
- **THEN** it SHALL copy a binary from the repo's `bin/emb` or download a release artifact
- **THEN** it SHALL run `gem build emb-server.gemspec`
- **THEN** the exit code SHALL be 0
