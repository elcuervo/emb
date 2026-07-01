## 1. Create install.sh

- [x] 1.1 Create `install.sh` at repo root: platform detection, version resolution, download and extract
- [x] 1.2 Support `EMB_INSTALL_DIR` env var for custom install dir
- [x] 1.3 Print clear error for unsupported platforms

## 2. Add macOS arm64 ORT bundling to CI

- [x] 2.1 Modify `InitEnvironment` in `internal/onnx/runtime.go` to try executable directory for bundled ORT
- [x] 2.2 Replace GoReleaser with raw CI build: `CGO_LDFLAGS` with `-Wl,-rpath,@loader_path`
- [x] 2.3 Bundle `libonnxruntime.1.dylib` in the tarball alongside `emb`
- [x] 2.4 Name tarball `emb_<version>_darwin_arm64.tar.gz`
- [x] 2.5 Upload to GitHub release via `softprops/action-gh-release`

## 3. Add Linux arm64 cross-build to CI

- [x] 3.1 Add `release-linux-arm64` job using Docker buildx QEMU
- [x] 3.2 Package `emb` + `libonnxruntime.so*` into `emb_<version>_linux_arm64.tar.gz`
- [x] 3.3 Upload to GitHub release with `softprops/action-gh-release`

## 4. Unify Linux amd64 tarball naming

- [x] 4.1 Rename `emb-$TAG-linux-amd64.tar.gz` → `emb_<version>_linux_amd64.tar.gz`
- [x] 4.2 Add `-Wl,-rpath,\$ORIGIN` to CGO_LDFLAGS for standalone resolution
- [x] 4.3 Add `-ldflags` for version string in binary

## 5. Update release workflow structure

- [x] 5.1 Remove GoReleaser step from macOS CI (keep `.goreleaser.yaml` for local dev)
- [x] 5.2 Add `versions.env` sourcing and `TAG_NAME` to all new jobs
- [x] 5.3 Update Dockerfile to include rpath for cross-build extraction

## 6. Update README with install instructions

- [x] 6.1 Add `curl | bash` one-liner at the top of README
- [x] 6.2 Update quick-start section to reference `curl | bash` as the primary install path

## 7. Verify

- [x] 7.1 `go vet ./...` — passes (CGo-free packages)
- [x] 7.2 `go build ./internal/onnx/...` — compiles
- [x] 7.3 Release tarball naming verified across all jobs