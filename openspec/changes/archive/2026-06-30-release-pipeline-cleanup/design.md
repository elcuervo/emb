## Context

The current release workflow has confusing job naming (`release-linux-docker-arm64` builds binaries, `release-linux-docker-amd64` builds Docker). The pipeline also mixes concerns — binary packaging and Docker image building happen in the same jobs. A clean separation makes the flow obvious and maintainable.

## Goals / Non-Goals

**Goals:**
- Clear job names that describe what they produce: `build-linux-amd64`, `build-linux-arm64`, `build-darwin-arm64`, `docker`
- Sequential stages: test → binary builds → Docker build
- All three binary tarballs (linux-amd64, linux-arm64, darwin-arm64) attached to GitHub Release
- Single `docker` job pushing a multi-arch manifest (`linux/amd64` + `linux/arm64`)

**Non-Goals:**
- Changing how binaries are built (native for amd64 and darwin, Docker QEMU for arm64)
- Changing artifact naming or release process
- Adding new platforms (Windows, 32-bit)

## Decisions

### Decision: Two-stage pipeline

```
test
  ├─ build-linux-amd64 ─────────────────┐
  ├─ build-linux-arm64 (via Docker) ─────┤
  └─ build-darwin-arm64 ─────────────────┤
                                         ▼
                              ┌──────────────────────┐
                              │  attach to GH Release │
                              └──────────────────────┘
                                         ▼
                                     ┌────────┐
                                     │ docker │
                                     │ (multi │
                                     │ -arch) │
                                     └────────┘
```

### Decision: Build job naming

| Job | What | How | Depends on |
|-----|------|-----|------------|
| `build-linux-amd64` | Native Go build on Ubuntu runner | `go build` + CGo | `test` |
| `build-linux-arm64` | Cross-build via Docker QEMU | `docker buildx build --output type=local` | `test` |
| `build-darwin-arm64` | Native Go build on macOS runner | `go build` + CGo | `test` |

### Decision: Single `docker` job for multi-arch push

One `docker` job builds and pushes both platforms in a single command:

```yaml
docker buildx build --platform linux/amd64,linux/arm64 --push
```

This produces a single multi-arch manifest tagged as `{version}` and `latest`. No per-platform Docker tags needed — the manifest handles architecture selection at pull time.

The `docker` job depends on all three `build-*` jobs (ensures binaries compiled before pushing).

### Decision: Dependency chain

- All `build-*` jobs depend on `test`
- `docker` depends on all `build-*` jobs (all binaries validated before Docker push)

## Risks / Trade-offs

- **arm64 binary build via QEMU is slow** — QEMU emulation on amd64 runner takes longer than native. Mitigation: docker buildx cache via GHA.
- **Docker build rebuilds from scratch** — even though binaries were already built natively, the Dockerfile builds its own Go binary inside the container. Mitigation: the Dockerfile build is cached by GHA layer cache.
- **Softprops uploads in each build job** — if multiple build jobs finish simultaneously, race condition on release upload. Mitigation: `fail_on_unmatched_files: true` catches issues early.
