# runtime-quality-validation Specification

## Purpose
Specifies the regression and static-quality bar for the runtime lifecycle boundaries: admission, drain, pool construction and closure, batch bounds, and download publication are covered by focused deterministic tests, and the existing vet and dead-code checks keep passing.

## Requirements
### Requirement: Runtime quality checks
The repaired runtime boundaries SHALL have focused regression tests, and the repository SHALL pass its existing full vet and dead-code checks without expanding CI infrastructure.

#### Scenario: Runtime lifecycle regression

- **WHEN** pool construction, closure, concurrent model initialization, request admission, shutdown, batch bounds, or download publication regresses
- **THEN** a focused deterministic test SHALL fail

#### Scenario: Static quality regression

- **WHEN** the completed implementation is validated
- **THEN** `go vet ./...` and `just deadcode` SHALL both pass inside `nix develop`
