## Context

The current release pipeline (`release.yml`) is a single Docker build/push step triggered on GitHub release publish. There are no binary downloads, no SHA256 checksums, no changelog generation. Adding GoReleaser brings a standard Go release workflow that the ecosystem expects.

The key challenge: `emb` uses CGo (ONNX Runtime shared library, libtokenizers static library). GoReleaser cannot cross-compile CGo binaries without the target platform's native libraries. The solution is a Docker-based build approach.

## Goals / Non-Goals

**Goals:**
- Replace manual Docker build with GoReleaser-managed release pipeline
- Publish multi-arch Docker images (linux/amd64, linux/arm64) to Docker Hub
- Publish binary archives + checksums to GitHub Releases for Linux AND macOS (darwin/amd64, darwin/arm64)
- Generate changelog from conventional commits
- Clear CI staging: tests → performance check → build → ship
- Allow `goreleaser --snapshot` for local testing

**Non-Goals:**
- Windows builds (not a target deployment platform)
- Homebrew tap or other package managers (feasible later)
- Signing artifacts (can be added later)
- macOS Docker builds (Docker on macOS CI is not practical; Docker images are Linux-only)

## Decisions

### CI pipeline stages

The release workflow has four sequential stages:

```
 1. Test          go test ./... (CGo-free packages on ubuntu)
 2. Performance   just verify-embeddings (check compat + cos-sim)
       ↓  (all pass)
 3. Build         Build binaries for all targets (Linux via Docker builder, macOS natively)
 4. Ship          GoReleaser archives + Docker multi-arch → GitHub Release + Docker Hub
```

Each stage runs only if the previous succeeds. The workflow is triggered on `release: [published]`.

### macOS binary builds

macOS builds require ONNX Runtime + libtokenizers as native libraries. In CI:

- **Linux binaries**: Built via the existing Dockerfile (multi-stage Docker builder). Works for amd64 + arm64.
- **macOS binaries**: Built on `macos-latest` runner using Homebrew-provided ONNX Runtime + downloaded libtokenizers.

GoReleaser's build matrix per platform:

| Platform | CI Runner | CGo deps | Approach |
|----------|-----------|----------|----------|
| linux/amd64 | ubuntu-latest | ORT + libtokenizers | Dockerfile builder, extract binary |
| linux/arm64 | ubuntu-latest (QEMU) | ORT + libtokenizers | Dockerfile builder (cross-arch), extract binary |
| darwin/amd64 | macos-latest | Homebrew ORT + libtokenizers | Native build (x86_64) |
| darwin/arm64 | macos-latest | Homebrew ORT + libtokenizers | Native build (arm64) |

For macOS builds in CI:
1. `brew install onnxruntime`
2. Download `libtokenizers.darwin-{arch}.tar.gz`
3. Build with `CGO_ENABLED=1 go build`

GoReleaser's `builds.hooks.pre` can handle dependency setup per platform.

### CGo build strategy: Docker-based builder

GoReleaser supports custom build hooks. The approach for CGo:

1. A `Dockerfile.builder` image pre-built with ONNX Runtime + libtokenizers
2. GoReleaser uses `before.hooks` to build inside this image
3. Or simpler: build the binary in CI before GoReleaser runs, use `builds.binary` to point to it

The simplest approach: **build the binary in the CI workflow before GoReleaser, then use GoReleaser for archiving/release/Docker**.

```
CI Workflow:
  1. Build emb binary for linux/amd64 + linux/arm64 (using existing Dockerfile)
  2. Extract binaries from builder stage
  3. Run GoReleaser:
     - Takes pre-built binaries
     - Creates tar.gz archives
     - Generates checksums
     - Builds Docker images (using Dockerfile.goreleaser — just COPYs binary + ONNX libs)
     - Creates multi-arch manifest
     - Publishes to GitHub Releases
     - Pushes to Docker Hub
```

### .goreleaser.yaml structure

```yaml
project_name: emb
release:
  github:
    owner: elcuervo
    name: emb
  prerelease: auto

builds:
  - id: emb
    binary: emb
    goos:
      - linux
    goarch:
      - amd64
      - arm64
    hooks:
      pre: # skip, binary built in CI workflow step

archives:
  - id: emb
    builds:
      - emb
    format: tar.gz
    files:
      - README.md

checksum:
  name_template: "checksums.txt"

dockers:
  - dockerfile: Dockerfile.goreleaser
    use: buildx
    build_flag_templates:
      - "--platform=linux/amd64"
      - "--label=org.opencontainers.image.version={{ .Version }}"
    image_templates:
      - "elcuervo/emb:{{ .Version }}-amd64"
      - "elcuervo/emb:latest-amd64"
  - dockerfile: Dockerfile.goreleaser
    use: buildx
    build_flag_templates:
      - "--platform=linux/arm64"
      - "--label=org.opencontainers.image.version={{ .Version }}"
    image_templates:
      - "elcuervo/emb:{{ .Version }}-arm64v8"
      - "elcuervo/emb:latest-arm64v8"

docker_manifests:
  - name_template: "elcuervo/emb:{{ .Version }}"
    image_templates:
      - "elcuervo/emb:{{ .Version }}-amd64"
      - "elcuervo/emb:{{ .Version }}-arm64v8"
  - name_template: "elcuervo/emb:latest"
    image_templates:
      - "elcuervo/emb:latest-amd64"
      - "elcuervo/emb:latest-arm64v8"

changelog:
  sort: asc
  filters:
    exclude:
      - "^docs:"
      - "^test:"
      - "^chore:"
      - "^ci:"
```

### Dockerfile.goreleaser (minimal runtime image)

A separate Dockerfile for GoReleaser that expects the binary and ONNX Runtime .so at known paths:

```dockerfile
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY emb /usr/local/bin/emb
COPY libonnxruntime.so* /usr/lib/

RUN ldconfig

RUN mkdir -p /etc/emb && echo 'listen: ":6379"\nmodels: {}' > /etc/emb/config.yaml

EXPOSE 6379
ENTRYPOINT ["emb", "-config", "/etc/emb/config.yaml"]
```

### release.yml structure

```yaml
name: Release
on:
  release:
    types: [published]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
      - name: Install libtokenizers
        run: ... # download for linux-x86_64
      - name: Test
        run: CGO_ENABLED=1 go test ./...
      - name: Verify embeddings
        run: just verify-embeddings

  release:
    needs: [test]
    runs-on: ubuntu-latest
    strategy:
      matrix:
        platform: [linux/amd64, linux/arm64]
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
      - uses: docker/setup-qemu-action@v3
      - uses: docker/setup-buildx-action@v3
      - uses: docker/login-action@v3
        with:
          username: elcuervo
          password: ${{ secrets.DOCKER_PASSWORD }}
      - name: Build binary for ${{ matrix.platform }}
        run: docker buildx build --platform ${{ matrix.platform }} --output=out/ ...
      - uses: goreleaser/goreleaser-action@v6
        with:
          args: release --clean

  release-macos:
    needs: [test]
    runs-on: macos-latest
    strategy:
      matrix:
        arch: [amd64, arm64]
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
      - name: Install ONNX Runtime
        run: brew install onnxruntime
      - name: Install libtokenizers
        run: ... # download for darwin-${{ matrix.arch }}
      - name: Build and release
        uses: goreleaser/goreleaser-action@v6
        with:
          args: release --clean
```

## Risks / Trade-offs

- [GoReleaser adds CI complexity] → Standardized release pipeline is worth the one-time setup. GoReleaser is the standard for Go projects.
- [Dockerfile.goreleaser duplicates runtime setup] → Small duplication vs keeping the dev Dockerfile. Acceptable.
- [CI builds the binary twice: once for archiving, once for Docker] → The same binary is reused. GoReleaser's `dockers` config uses the pre-built binary from its build step.
- [Tags like pre-release] → `prerelease: auto` handles this. Only full releases get the "latest" tags.
