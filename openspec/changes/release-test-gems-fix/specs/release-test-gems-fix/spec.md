## ADDED Requirements

### Requirement: Clean release when all secrets present

When all required secrets (DOCKER_PASSWORD, rubygems) are configured, the release workflow SHALL complete with all jobs passing.

#### Scenario: Tag push triggers clean release

- **GIVEN** a tag `v*` is pushed
- **WHEN** the release workflow runs
- **THEN** `test` SHALL pass
- **THEN** `build-linux-amd64` SHALL produce a tarball and upload to the release
- **THEN** `build-linux-arm64` SHALL produce a tarball and upload to the release
- **THEN** `build-darwin-arm64` SHALL produce a tarball and upload to the release
- **THEN** `docker` SHALL build and push multi-arch images to Docker Hub
- **THEN** `test-gems` SHALL run the RSpec suite against the downloaded binary
- **THEN** `release-emb` SHALL build and publish the `emb` gem to rubygems.org
- **THEN** `release-emb-server` SHALL build and publish the `emb-server` gem to rubygems.org

### Requirement: test-two-models.yaml is tracked

The `test-two-models.yaml` file SHALL be tracked in git so the `test-gems` job can locate it at checkout.

#### Scenario: File present after checkout

- **WHEN** a runner checks out the repository
- **THEN** `test-two-models.yaml` SHALL exist in the working directory

### Requirement: Ruby is available for gem jobs

The `test-gems`, `release-emb`, and `release-emb-server` jobs SHALL set up Ruby via `ruby/setup-ruby@v1` before running gem commands.

#### Scenario: bundle available in test-gems

- **WHEN** `test-gems` runs
- **THEN** `bundle install` SHALL succeed in `gems/emb/`
- **THEN** `bundle exec rake` SHALL pass

#### Scenario: gem available in release-emb

- **WHEN** `release-emb` runs
- **THEN** `gem build emb.gemspec` SHALL succeed
- **THEN** `gem install --local` SHALL succeed
- **THEN** `gem push` SHALL publish to rubygems.org

#### Scenario: gem available in release-emb-server

- **WHEN** `release-emb-server` runs
- **THEN** `gem build emb-server.gemspec` SHALL succeed
- **THEN** `gem install --local` SHALL succeed
- **THEN** `gem push` SHALL publish to rubygems.org
