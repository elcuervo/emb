# project-versioning

## Purpose

Specifies a unified versioning scheme where the root `VERSION` file is the single source of truth for the Go binary and both Ruby gems.

## Requirements

### Requirement: Root VERSION file

The repo SHALL contain a `VERSION` file at the root containing the current project version as a plain text string (no newline at end, or single line with newline).

#### Scenario: File exists

- **WHEN** the repo is cloned
- **THEN** `VERSION` SHALL exist at the repo root
- **THEN** it SHALL contain a semver string (e.g., `0.1.0`)

### Requirement: All artifacts use same version

The Go binary, `gems/emb`, and `gems/emb-server` SHALL all use the version from the root `VERSION` file.

#### Scenario: just build reads VERSION

- **WHEN** `just build` runs
- **THEN** the Go binary SHALL have its version ldflag set from root `VERSION`
- **THEN** `./bin/emb -version` SHALL print the root VERSION value

#### Scenario: gem build reads VERSION

- **WHEN** `gem build emb.gemspec` runs in `gems/emb/`
- **THEN** the gem version SHALL be read from root `VERSION`
- **WHEN** `gem build emb-server.gemspec` runs in `gems/emb-server/`
- **THEN** the gem version SHALL be read from root `VERSION`

### Requirement: CI injects tag version

The release workflow SHALL overwrite the root `VERSION` file with the release tag version before building artifacts.

#### Scenario: Tag push triggers version write

- **WHEN** a tag `v0.2.0` is pushed
- **THEN** the workflow SHALL write `0.2.0` to root `VERSION`
- **THEN** all artifacts SHALL be built with version `0.2.0`
