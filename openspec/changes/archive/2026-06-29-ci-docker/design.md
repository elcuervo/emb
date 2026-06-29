## Context

The Docker build is manual-only. CI doesn't exist. The image is 172MB (97MB Debian base + 60MB ONNX Runtime + 6MB binary + 9MB overhead) — reasonable for a server binary with C dependencies.

## Goals / Non-Goals

**Goals:**
- CI runs on every push: lint + test
- Release workflow builds and pushes multi-arch Docker image
- Image size verified (~172MB) and documented in the release workflow
- `go build` tested in CI (using setup-go action, no nix dependency)
- Docker Hub credentials configured via GitHub secrets

**Non-Goals:**
- Running ONNX Runtime / model inference in CI (requires ORT shared library)
- Cross-compilation without QEMU (CGo requires native toolchain per arch)
- Nix-based CI (CI uses standard Go + Docker, not nix)

## Decisions

### CI without ONNX Runtime

The CI runs `go build ./...` with `CGO_ENABLED=0` (no ONNX Runtime needed for compilation). Tests that require CGo (the server integration tests with real sessions) are skipped with `-short`. The lint and formatting checks work without CGo.

This means CI won't run the full end-to-end test suite. The full test requires a nix environment with ONNX Runtime. That's acceptable — the CI catches compilation errors, formatting issues, and pure-Go test failures.

### Release workflow with Docker buildx

The release workflow:
1. Check out code
2. Set up Docker Buildx with QEMU for multi-arch
3. Log in to Docker Hub using secrets
4. Run `docker buildx build --platform linux/amd64,linux/arm64 --push -t elcuervo/emb-server:<tag> -t elcuervo/emb-server:latest .`

The tag is the Git tag name (e.g., `v0.1.0`).

### Image size at 172MB

The current image is 172MB with the ORT shared library at 60MB. This is acceptable — the base `debian:bookworm-slim` is 97MB. Future size optimization (using `distroless` or `alpine`) is blocked by ORT's glibc requirement.

## Risks / Trade-offs

- [CI skips CGo tests] → The compilation step (`go build`) still passes because CGo is disabled. Runtime CGo errors would be caught during development, not CI.
- [Docker Hub rate limits] → The release workflow pushes once per release. Anonymous pulls during build may hit rate limits. Authenticated pulls have higher limits.
- [Buildx QEMU is slow for arm64] → Expected 5-10 minutes for the arm64 build. Acceptable for release builds.
