## Why

Installing `emb` requires Go, CGo, ONNX Runtime, and libtokenizers. A Ruby gem distributes the precompiled binary for each platform with a single `gem install emb-server` command. Users get the exact same `emb` Go server on their PATH with zero build steps.

## What Changes

- Create `gems/emb-server/` directory with a multi-platform Ruby gem structure
- Platform-specific gems: `arm64-darwin`, `x86_64-darwin`, `aarch64-linux`, `x86_64-linux`
- Each gem bundles:
  - Precompiled `emb-binary` (Go binary compiled against ORT v1.27.0, statically linked against libtokenizers)
  - `bin/emb` wrapper script that finds the `onnxruntime` gem's shared library and exec's the binary with `-ort-lib`
  - `lib/emb-server/version.rb`
- Add `onnxruntime` gem as a runtime dependency (provides the `.dylib`/`.so` at runtime)
- Add release pipeline step to build platform gems from the CI-built Go binaries

## Capabilities

### New Capabilities
- `emb-server-distribution`: Distribution gem for the `emb` Go server. Single-command install, platform-specific gems, auto-resolves ONNX Runtime library path.

### Modified Capabilities
- `release-pipeline`: Add job to build and push platform-specific gems as part of the release workflow

## Impact

| File | Change |
|------|--------|
| `gems/emb-server/` | **Added** — gem directory with gemspec, bin, lib |
| `.github/workflows/release.yml` | Add `release-gem` job that builds emb-binary per arch and publishes platform gems |
