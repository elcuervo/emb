## Context

The server requires CGo for ONNX Runtime, which means Docker buildx with QEMU emulation is the most practical multi-arch approach. Cross-compilation with CGo is fragile (different toolchains per arch), while QEMU emulates the target architecture natively within Docker.

## Goals / Non-Goals

**Goals:**
- Single `just docker-push` builds and pushes multi-arch image to Docker Hub
- Image runs on linux/amd64 and linux/arm64
- Minimal runtime image size (no build tools, just the binary + runtime deps)
- Image includes CA certificates for HTTPS model downloads at runtime
- Tagged with both `latest` and `$(git rev-parse --short HEAD)`

**Non-Goals:**
- GitHub Actions CI workflow (future)
- ARM64 emulation for development (use native build)
- Including models in the image (mount or download at startup)

## Decisions

### Multi-stage Dockerfile

```
Stage 1 (builder): golang:1.25-bookworm + ONNX Runtime C lib
    → Build Go binary with CGO_ENABLED=1

Stage 2 (runtime): debian:bookworm-slim
    → Copy binary + libonnxruntime.so + ca-certificates
    → ~150MB final image
```

ONNX Runtime 1.21.0 pre-built shared libraries are downloaded from GitHub releases. Version is pinned in the Dockerfile. The `ldconfig` step registers the shared library.

### Multi-arch via buildx

`docker buildx build --platform linux/amd64,linux/arm64 --push ...`

This uses QEMU binfmt registrations to emulate the target architecture. The `Dockerfile` uses `ARG TARGETARCH` to select the correct ONNX Runtime binary (`x86_64` vs `aarch64`).

### just targets

```makefile
docker-build   # Build multi-arch image
docker-push    # Build and push to Docker Hub
```

Both assume Docker is installed and buildx is configured. The push target logs a warning if not authenticated.

## Risks / Trade-offs

- [QEMU emulation is slow for arm64 builds on x86] → Initial arm64 build takes 5-10 minutes. Caching and buildx's layer cache help.
- [ONNX Runtime ABI compatibility] → The Go binding (`onnxruntime_go`) must match the ORT C library version. Pinned to 1.21.0 in the Dockerfile. If the Go binding upgrades, the Dockerfile must be updated.
- [Large image due to ONNX Runtime] → The .so file is ~50MB. Total image is ~150MB which is acceptable for a server binary.
- [Docker Hub rate limits] → Anonymous pulls are limited. Users may need `docker login` before building.
