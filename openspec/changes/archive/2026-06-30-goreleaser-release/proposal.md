## Why

The current release workflow (`release.yml`) only publishes a Docker image to Docker Hub via `docker/build-push-action`. There is no binary release, no checksum file, no changelog, and no standardized release pipeline. GoReleaser provides a single configuration for building binaries, creating archives, signing artifacts, generating changelogs, and publishing to GitHub Releases — all alongside Docker image publishing.

## What Changes

- Add `.goreleaser.yaml` configuration for building, archiving, and releasing the `emb` binary
- Update `release.yml` to use GoReleaser's GitHub Action instead of manual Docker build
- Add a minimal `Dockerfile.goreleaser` for GoReleaser's Docker integration (expects pre-built binary)
- Keep the existing `Dockerfile` for development and manual builds
- GoReleaser handles:
  - Linux amd64 + arm64 binary builds (via Docker-based builder or proper CGo cross-compilation setup)
  - Archive creation (tar.gz with binary)
  - Checksum file generation
  - GitHub Release creation with changelog
  - Multi-arch Docker image publishing (via GoReleaser's `dockers` config)
  - Docker manifest creation for multi-arch

## Capabilities

### New Capabilities

- `ci-release`: GoReleaser-powered CI/CD pipeline with binary releases, archives, checksums, and multi-arch Docker images

### Modified Capabilities

- (none — current behavior is not specified at spec level)

## Impact

| File | Change |
|------|--------|
| `.goreleaser.yaml` | **Created** — GoReleaser build/archive/release/docker config |
| `Dockerfile.goreleaser` | **Created** — minimal runtime image for GoReleaser Docker builds (binary + ONNX Runtime .so) |
| `.github/workflows/release.yml` | **Rewritten** — use `goreleaser-action` for both binary release and Docker publishing |
| `Dockerfile` | Unchanged (kept for dev/manual use) |
| `justfile` | Add `release-dry-run` recipe for testing GoReleaser locally |
