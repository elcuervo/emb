## ADDED Requirements

### Requirement: GoReleaser release pipeline

The project SHALL use GoReleaser for managing releases, building binaries, and publishing Docker images.

#### Scenario: GitHub Release created

- **WHEN** a GitHub Release is published
- **THEN** the CI pipeline SHALL run GoReleaser
- **THEN** GoReleaser SHALL build Linux amd64 and arm64 binaries
- **THEN** GoReleaser SHALL create tar.gz archives with checksums
- **THEN** GoReleaser SHALL upload archives to the GitHub Release

#### Scenario: Multi-arch Docker image published

- **WHEN** a GitHub Release is published
- **THEN** the CI pipeline SHALL build and push Docker images for linux/amd64 and linux/arm64
- **THEN** the CI pipeline SHALL create a multi-arch manifest tagged `elcuervo/emb:latest` and `elcuervo/emb:<version>`

#### Scenario: Local dry-run

- **WHEN** a developer runs `goreleaser --snapshot`
- **THEN** GoReleaser SHALL build binaries and create archives in the `dist/` directory without publishing
