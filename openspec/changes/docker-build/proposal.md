## Why

The server is currently only buildable via `nix develop`, limiting deployment to machines with nix. A Docker image enables deployment anywhere — bare metal, VPS, Kubernetes, or serverless containers. Multi-arch support (linux/amd64 + linux/arm64) covers both x86 servers and ARM instances like AWS Graviton or Apple Silicon CI runners.

## What Changes

- `Dockerfile` using multi-stage build with ONNX Runtime shared library
- `.dockerignore` excluding models/, bin/, and IDE files
- `just docker-build` target: builds multi-arch image via `docker buildx`
- `just docker-push` target: builds and pushes to `elcuervo/emb-server:latest`
- Image tagged with both `latest` and git commit hash
- Minimal runtime image (~150MB): Go binary + ONNX Runtime shared lib + ca-certificates

## Capabilities

### New Capabilities

- `docker-build`: Multi-arch Docker image build and push infrastructure

### Modified Capabilities

None — Docker packaging is transparent to the server behavior.

## Impact

Files: `Dockerfile`, `.dockerignore`, `justfile` (new targets). No changes to Go source, config format, or RESP protocol.
