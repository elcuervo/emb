## ADDED Requirements

### Requirement: justfile with development tasks

The project SHALL provide a `justfile` at the repository root with targets for common development workflows.

#### Scenario: just --list shows all targets

- **WHEN** user runs `just --list`
- **THEN** output includes `format`, `lint`, `test`, `bench`, `baseline`, `dev`, `download-model`, `clean`

#### Scenario: just format formats all Go code

- **WHEN** user runs `just format`
- **THEN** all `.go` files are formatted with `gofmt` and `goimports`

#### Scenario: just lint runs all linters

- **WHEN** user runs `just lint`
- **THEN** `golangci-lint` runs against all Go packages and reports any issues

#### Scenario: just test runs all tests

- **WHEN** user runs `just test`
- **THEN** `go test ./...` is executed

#### Scenario: just bench runs all benchmarks

- **WHEN** user runs `just bench`
- **THEN** `go test -bench=. ./...` is executed

#### Scenario: just baseline captures benchmark results

- **WHEN** user runs `just baseline`
- **THEN** benchmark output is saved to `benchmark-baseline.txt`

### Requirement: Go formatting and linting tools

The project SHALL use `gofmt`, `goimports`, and `golangci-lint` for code quality.

#### Scenario: golangci-lint configuration

- **WHEN** user runs `golangci-lint run`
- **THEN** it reads `.golangci.yml` and checks all Go files using `staticcheck`, `govet`, `gofmt`, and `goimports` linters

### Requirement: Project hygiene files

The project SHALL include `.gitignore` and `.golangci.yml` at the repository root.

#### Scenario: gitignore ignores models and build artifacts

- **WHEN** git operations are performed
- **THEN** `models/`, `/tmp/emb`, and IDE-specific files are ignored

#### Scenario: flake dev shell includes tools

- **WHEN** user enters `nix develop`
- **THEN** `just`, `golangci-lint`, `goimports` are available in PATH
