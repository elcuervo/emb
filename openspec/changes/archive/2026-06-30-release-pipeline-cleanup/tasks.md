## 1. Rename Binary Build Jobs

- [x] 1.1 Rename `release-linux` → `build-linux-amd64` (native Go build on Ubuntu, amd64 binary + libonnxruntime.so tarball)
- [x] 1.2 Rename `release-linux-docker-arm64` → `build-linux-arm64` (Docker QEMU build, arm64 binary + libonnxruntime.so tarball)
- [x] 1.3 Rename `release-macos` → `build-darwin-arm64` (native Go build on macOS, arm64 binary + libonnxruntime.dylib tarball)

## 2. Create Docker Job

- [x] 2.1 Rename existing `release-linux-docker-amd64` job to `docker`. Single multi-arch build: `--platform linux/amd64,linux/arm64 --push`. Tag as `{version}` and `latest`.
- [x] 2.2 `docker` job has `if: github.event_name != 'workflow_dispatch'` guard — only runs on real releases

## 3. Wire Dependencies

- [x] 3.1 All `build-*` jobs depend on `test`
- [x] 3.2 `docker` job depends on `build-linux-amd64`, `build-linux-arm64`, and `build-darwin-arm64`
