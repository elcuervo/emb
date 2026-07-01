# gems-release-lifecycle

## Purpose

Specifies the release lifecycle requirements for both Ruby gems, including gem testing and publication during the release workflow.

## Requirements

### Requirement: Gem testing in release pipeline

The release workflow SHALL test both gems before publishing.

#### Scenario: Gems tested with running server

- **WHEN** the release workflow runs
- **THEN** a `test-gems` job SHALL start after all `build-*` jobs complete
- **THEN** it SHALL download the `emb` binary from the release artifacts
- **THEN** it SHALL start the `emb` server with a test configuration
- **THEN** it SHALL run `cd gems/emb && bundle exec rake` (RSpec suite)
- **THEN** it SHALL build `gems/emb-server` gem to validate it
- **THEN** it SHALL stop the server
- **THEN** the exit code SHALL be 0

### Requirement: Both gems published on release

The release workflow SHALL publish both `gems/emb` (Ruby client) and `gems/emb-server` (distribution) to rubygems.org.

#### Scenario: emb gem published

- **WHEN** the release workflow runs
- **THEN** `gems/emb` SHALL be built as a platform-independent `ruby` gem
- **THEN** it SHALL be pushed to rubygems.org

#### Scenario: emb-server gems published per platform

- **WHEN** the release workflow runs
- **THEN** `gems/emb-server` SHALL be built for each supported platform (`arm64-darwin`, `x86_64-linux`, `aarch64-linux`)
- **THEN** each platform variant SHALL be pushed to rubygems.org
