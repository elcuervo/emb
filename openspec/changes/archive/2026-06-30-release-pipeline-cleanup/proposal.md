## Why

The release pipeline currently has unclear job naming and splits responsibility in confusing ways. `release-linux` builds amd64 binaries while `release-linux-docker-arm64` builds arm64 binaries and `release-linux-docker-amd64` pushes Docker images. A clean pipeline should make the flow obvious: test → build binaries → build Docker images, with job names that reflect what they actually produce.

## What Changes

- Rename jobs to follow a `build-<platform>` and `docker-<platform>` convention
- Separate binary build jobs from Docker build job explicitly
- Run all three binary builds in parallel after tests pass (linux-amd64, linux-arm64, darwin-arm64)
- Run a single `docker` job after all binary builds complete that pushes a multi-arch manifest (`linux/amd64` + `linux/arm64`) in one command

## Capabilities

### New Capabilities
- `release-pipeline`: Clean CI/CD pipeline with named stages: test → build-binaries → build-docker-images → push

### Modified Capabilities
- (none — this is a new workflow spec)

## Impact

| File | Change |
|------|--------|
| `.github/workflows/release.yml` | Rename jobs, restructure dependencies, single Docker multi-arch push |
