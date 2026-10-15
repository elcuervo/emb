# Spec Delta

## MODIFIED Requirements

### Requirement: Multi-arch Docker image

The project SHALL provide a `Dockerfile` that builds a multi-arch Docker image for linux/amd64 and linux/arm64. The image SHALL include the `emb` server binary, the `emb-top` dashboard binary and the `emb-stress` load generator, the latter two built as static (CGo-free) binaries.

#### Scenario: Image builds for amd64

- **WHEN** `docker buildx build --platform linux/amd64` is run
- **THEN** the image contains the emb binary, the emb-top binary, the emb-stress binary, ONNX Runtime shared library, and CA certificates

#### Scenario: Image builds for arm64

- **WHEN** `docker buildx build --platform linux/arm64` is run
- **THEN** the image contains the emb binary, the emb-top binary, the emb-stress binary, and ONNX Runtime shared library compiled for aarch64

#### Scenario: emb-top runs from the image

- **WHEN** a container is started from the image against a running emb node
- **THEN** `/usr/local/bin/emb-top` starts and renders the dashboard

#### Scenario: emb-stress runs from the image

- **WHEN** a container is started from the image with a server configuration mounted against a running emb node
- **THEN** `/usr/local/bin/emb-stress` drives load against that node and reports the run's levels
