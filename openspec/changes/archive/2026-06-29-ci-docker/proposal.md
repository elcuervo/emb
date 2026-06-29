## Why

The Docker image builds successfully but hasn't been validated for multi-arch, and there's no automated CI pipeline. Pushes to Docker Hub require manual steps. A GitHub Actions workflow ensures tests pass on every commit and releases are automatically pushed as multi-arch Docker images.

## What Changes

- Validate multi-arch Docker build: measure final image size (172MB), verify arm64 target compiles
- Create `.github/workflows/ci.yml`: runs `just lint`, `just test` on push/PR
- Create `.github/workflows/release.yml`: runs tests, builds multi-arch Docker image, pushes to Docker Hub on release
- Configure Docker Hub secrets (`DOCKER_USERNAME`, `DOCKER_PASSWORD`) documented in the workflow
- Test and verify multi-arch push works end-to-end

## Capabilities

### New Capabilities

- `ci-docker`: GitHub Actions CI pipeline and automated multi-arch Docker pushes

## Impact

Files: `.github/workflows/ci.yml`, `.github/workflows/release.yml`, `justfile` (if needed for CI-specific targets). The existing `Dockerfile` is unchanged.
