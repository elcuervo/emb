## Why

Installing `emb` today means either Docker, Nix, or building from source with native CGo dependencies. There's no way to just download a binary and run it. A `curl | bash` one-liner removes the biggest barrier to adoption: "how do I install this thing?"

## What Changes

- **Release pipeline overhaul**: Replace the hybrid GoReleaser + raw CI approach with raw CI builds for all three targets, producing self-contained tarballs that bundle `libonnxruntime`
- **macOS arm64 builds**: Add `libonnxruntime.dylib` to the tarball and rewrite the binary's load path with `install_name_tool` so it finds the dylib at `@loader_path` (same directory as itself)
- **Linux arm64 builds**: Add cross-build via Docker QEMU (or native ARM runner) producing `emb` + `libonnxruntime.so*`
- **Unified tarball naming**: All platforms produce `emb_<version>_<os>_<arch>.tar.gz`
- **Install script**: `install.sh` — detects platform, resolves latest GitHub release, downloads and extracts to `/usr/local/bin` (or `$EMB_INSTALL_DIR`)
- **README update**: Add the `curl | bash` one-liner

## Capabilities

### New Capabilities
- `standalone-distribution`: Self-contained platform-specific tarballs with bundled ONNX Runtime, published as GitHub Release assets, installable via a `curl | bash` script

### Modified Capabilities
- (none — the binary itself, its CLI, config format, and behavior are unchanged)

## Impact

| File | Change |
|------|--------|
| `.github/workflows/release.yml` | Replace GoReleaser macOS job with raw CI build (like Linux), add Linux arm64 build, bundle ORT in all platform tarballs, unify naming to `emb_<version>_<os>_<arch>.tar.gz` |
| `.goreleaser.yaml` | Remove (no longer used) — or keep for snapshot builds only |
| `install.sh` | New file: platform detection, latest release resolution, download and extract |
| `.github/workflows/ci.yml` | No change |
| `README.md` | Add `curl https://.../install.sh \| sh` one-liner |
