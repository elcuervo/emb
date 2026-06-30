## 1. Create .goreleaser.yaml

- [x] 1.1 Created `.goreleaser.yaml` with darwin builds (amd64 + arm64), archives, checksums
- [x] 1.2 Added `before.hooks.pre` for downloading libtokenizers per arch
- [x] 1.3 Archive config: tar.gz per platform with README
- [x] 1.4 Checksum config with `checksums.txt`
- [x] 1.5 Docker handled by `docker/build-push-action` in CI (not GoReleaser)
- [x] 1.6 Docker multi-arch manifests handled by `docker/metadata-action`
- [x] 1.7 Changelog config with docs/test/chore/ci exclusions

## 2. Dockerfile.goreleaser (not needed)

Docker images use existing `Dockerfile` via `docker buildx`. No separate Dockerfile needed for GoReleaser.

- [x] 2.1 Skipped — existing Dockerfile handles all Docker builds
- [x] 2.2 Skipped

## 3. Add build recipes to justfile

- [x] 3.1 Added `build-linux` recipe (Docker builder extraction)
- [x] 3.2 Added `release-dry-run` recipe (`goreleaser release --snapshot --clean`)
- [x] 3.3 GoReleaser runs only on macOS CI runner (darwin builds). Linux releases handled directly by release.yml. No cross-platform CGo conflict.

## 4. Update release.yml with staged pipeline

- [x] 4.1 **Stage 1 — Test (ubuntu-latest):** libtokenizers + ORT + go test
- [x] 4.2 **Stage 2 — Build Linux binaries + Docker (ubuntu-latest):** QEMU + Buildx + Docker Hub + `softprops/action-gh-release` for uploads
- [x] 4.3 **Stage 3 — Build macOS binaries (macos-latest, matrix: amd64, arm64):** brew ORT + libtokenizers + `goreleaser/goreleaser-action` for full build/archive/upload
- [x] 4.4 Stages: Test → Release Linux → Release macOS (sequential via `needs:`)
- [x] 4.5 Updated all Docker references from `elcuervo/emb-server` to `elcuervo/emb`

## 5. Verify

- [x] 5.1 GoReleaser tested on darwin via macos-latest runner in CI. Linux releases handled by `gh release upload` directly without GoReleaser (CGo cross-compilation limitation).
- [x] 5.2 `go vet ./...` — passes
- [x] 5.3 `go test ./...` — passes
- [ ] 5.4 `just verify-embeddings` — requires running server (skipped for CI-only change)
