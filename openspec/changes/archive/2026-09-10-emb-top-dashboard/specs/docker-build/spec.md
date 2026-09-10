## MODIFIED Requirements

### Requirement: Multi-arch Docker image

The project SHALL provide a `Dockerfile` that builds a multi-arch Docker image for linux/amd64 and linux/arm64. The image SHALL include both the `emb` server binary and the `emb-top` dashboard binary, the latter built as a static (CGo-free) binary.

#### Scenario: Image builds for amd64

- **WHEN** `docker buildx build --platform linux/amd64` is run
- **THEN** the image contains the emb binary, the emb-top binary, ONNX Runtime shared library, and CA certificates

#### Scenario: Image builds for arm64

- **WHEN** `docker buildx build --platform linux/arm64` is run
- **THEN** the image contains the emb binary, the emb-top binary, and ONNX Runtime shared library compiled for aarch64

#### Scenario: emb-top runs from the image

- **WHEN** a container is started from the image against a running emb node
- **THEN** `/usr/local/bin/emb-top` starts and renders the dashboard