## ADDED Requirements

### Requirement: Multi-arch Docker image

The project SHALL provide a `Dockerfile` that builds a multi-arch Docker image for linux/amd64 and linux/arm64.

#### Scenario: Image builds for amd64

- **WHEN** `docker buildx build --platform linux/amd64` is run
- **THEN** the image contains the emb binary, ONNX Runtime shared library, and CA certificates

#### Scenario: Image builds for arm64

- **WHEN** `docker buildx build --platform linux/arm64` is run
- **THEN** the image contains the emb binary and ONNX Runtime shared library compiled for aarch64

### Requirement: Docker push to elcuervo/emb

The project SHALL provide a `just docker-push` target that builds and pushes the multi-arch image.

#### Scenario: Push builds and pushes both architectures

- **WHEN** user runs `just docker-push`
- **THEN** the image is built for both linux/amd64 and linux/arm64 and pushed to Docker Hub as `elcuervo/emb:latest` and `elcuervo/emb:<git-sha>`

#### Scenario: Push warns if not authenticated

- **WHEN** user runs `just docker-push` without being logged into Docker Hub
- **THEN** the build fails with a clear authentication error
