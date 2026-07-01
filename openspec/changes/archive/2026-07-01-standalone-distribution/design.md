## Context

Currently `emb` is distributed via:
- **Docker Hub**: `elcuervo/emb` — multi-arch Docker images
- **GitHub Releases**: Linux tarballs with bundled `libonnxruntime.so*`, macOS tarballs from GoReleaser with no bundled ORT

The macOS tarball is broken standalone — the binary is linked against `/opt/homebrew/opt/onnxruntime/lib/libonnxruntime.dylib` but the dylib isn't included. Only users with ONNX Runtime installed via Homebrew can run it.

There's no install script, no `curl | bash` path, and no way to install `emb` without Docker or Nix.

## Goals / Non-Goals

**Goals:**
- Self-contained tarballs for all supported platforms (binary + libonnxruntime together)
- macOS arm64: bundle `libonnxruntime.dylib` and rewrite `@loader_path` so it finds the dylib in the same directory
- Linux arm64: add a cross-build job (QEMU via Docker buildx)
- Unified tarball naming: `emb_<version>_<os>_<arch>.tar.gz`
- `install.sh` script: detect platform, resolve latest release, download and extract
- `curl https://.../install.sh | sh` one-liner

**Non-Goals:**
- Darwin/amd64 (Intel Mac) builds
- Windows builds
- Statically linking ONNX Runtime
- Package manager distribution (deb, rpm, apk) beyond Homebrew
- Code changes to `emb` itself (binary, server, tokenizer, config — all unchanged)

## Decisions

### Build pipeline: raw CI jobs for all targets

Replace GoReleaser with explicit CI build steps, matching the existing Linux amd64 pattern. Each platform gets its own job:

| Job | Runner | ORT source | Bundle |
|-----|--------|------------|--------|
| `release-linux-amd64` | ubuntu-latest | Pre-built Microsoft tarball | `libonnxruntime.so*` |
| `release-linux-arm64` | ubuntu-latest + QEMU (Docker buildx) | Pre-built Microsoft tarball | `libonnxruntime.so*` |
| `release-darwin-arm64` | macos-latest | Homebrew (`brew install onnxruntime`) | `libonnxruntime.dylib` |

GoReleaser is removed for releases. It can be kept for local snapshot builds (`goreleaser release --snapshot`) but the release workflow owns all production artifacts.

### macOS ORT bundling: install_name_tool

The GoReleaser macOS binary (and any binary built against Homebrew's ORT) has the library path baked into its `LC_LOAD_DYLIB`:

```
% otool -L emb
emb:
    /opt/homebrew/opt/onnxruntime/lib/libonnxruntime.dylib
    /usr/lib/libSystem.B.dylib
    ...
```

After build, rewrite the load path to `@loader_path/libonnxruntime.dylib` so the loader looks in the same directory as the binary:

```bash
install_name_tool -change \
  /opt/homebrew/opt/onnxruntime/lib/libonnxruntime.dylib \
  @loader_path/libonnxruntime.dylib \
  emb
```

Then bundle both in the tarball:
```
emb_<version>_darwin_arm64.tar.gz
├── emb
└── libonnxruntime.dylib
```

`InitEnvironment` in `internal/onnx/runtime.go` tries bare filenames (`onnxruntime.dylib`, `libonnxruntime.dylib`) which works because `@loader_path` is searched first.

### Linux arm64: Docker QEMU cross-build

Use `docker buildx` with QEMU to build the Linux arm64 binary, matching the existing Linux amd64 approach:

1. Use the existing Dockerfile builder stage (multi-stage)
2. Run `docker buildx build --platform linux/arm64 --output type=local,dest=./dist/linux-arm64 .`
3. Extract `emb` and `libonnxruntime.so*` from the dist directory
4. Package into the tarball

Alternative: use a native ARM runner (GitHub provides `ubuntu-24.04-arm`). Docker QEMU is simpler to set up but slower. Native ARM runner is faster but requires tweaking the runner selector.

Decision: start with **Docker QEMU** (no changes to CI runner config needed), can migrate to native ARM runner later.

### Tarball naming convention

```
emb_<version>_<os>_<arch>.tar.gz
```

- `<version>`: semver without `v` prefix (e.g., `0.1.0`)
- `<os>`: `linux` or `darwin`
- `<arch>`: `amd64` or `arm64`

Examples: `emb_0.1.0_linux_amd64.tar.gz`, `emb_0.1.0_darwin_arm64.tar.gz`

This matches the GoReleaser default but applies to all platforms.

### Install script

The script (`install.sh`) will:

1. Detect `$(uname -s)` and `$(uname -m)` → target string
2. Call GitHub API to find the latest release tag
3. Construct the download URL
4. Download the tarball and extract to `/usr/local/bin` (or `$EMB_INSTALL_DIR`)

```bash
EMB_INSTALL_DIR="${EMB_INSTALL_DIR:-/usr/local/bin}"
```

So users who don't have write access to `/usr/local/bin` can do:
```bash
curl -fsSL https://.../install.sh | EMB_INSTALL_DIR=~/.local/bin sh
```

The script is hosted in the repo at `install.sh` and served via a raw GitHub URL or GitHub Pages.

### GoReleaser disposition

Keep `.goreleaser.yaml` for local development (`goreleaser release --snapshot --clean` produces a quick test build). Remove it from CI. The production release workflow bypasses it entirely.

## Risks / Trade-offs

### macOS rpath + InitEnvironment

The macOS binary is CGo-linked against ONNX Runtime, creating two resolution paths:
- **Process startup**: dynamic linker resolves `LC_LOAD_DYLIB` entries (via `@rpath` + `LC_RPATH`)
- **Runtime**: `onnxruntime_go` calls `dlopen` with a bare filename (via `SetSharedLibraryPath`)

`LC_RPATH` is searched for `@rpath`-based load commands at process start, but `dlopen` with a bare filename does NOT search `LC_RPATH`. To make both paths work:

1. **Build-time**: set `-Wl,-rpath,@loader_path` in `CGO_LDFLAGS` so the dynamic linker finds ORT at startup
2. **Post-build**: the dylib is bundled alongside the binary
3. **Code change** in `InitEnvironment`: try the executable's directory with versioned dylib names as a fallback, so `onnxruntime_go`'s `dlopen` can find it

This is the only code change to `emb` itself required for standalone distribution. The alternative (wrapper script + `DYLD_LIBRARY_PATH`) was rejected as inelegant.

### Linux rpath

On Linux, `DT_RPATH` is searched both for process-startup resolution AND for `dlopen` with bare filenames. Setting `DT_RPATH` to `$ORIGIN` at build time covers both paths:

```bash
CGO_LDFLAGS="-L/path -lonnxruntime -Wl,-rpath,'$ORIGIN'"
```

No code change needed for Linux.

### Bundled dylib naming

The tarball contains the versioned dylib name (`libonnxruntime.1.dylib` on macOS, `libonnxruntime.so.1` on Linux) because that matches the SONAME/install name in the ORT library. The binary references `@rpath/libonnxruntime.1.dylib`, and the rpath `@loader_path` resolves it.

- [Docker QEMU for Linux arm64 is slow] → ~5-10 min per build vs ~2 min native. Acceptable for release frequency. Can migrate to native ARM runner later.
- [macOS arm64 builds depend on `brew install onnxruntime`] → This adds ~1-2 min per run. `onnxruntime` is a standard Homebrew formula with good binary cache availability.
- [Double CI runner cost for Linux amd64] → Currently `release-linux` runs on ubuntu-latest. Adding `release-linux-arm64` doesn't affect the amd64 job.
- [install.sh has no signature verification] → Content is served over HTTPS. Users who need verification can download from GitHub Releases and check checksums. Can add GPG signing later.
