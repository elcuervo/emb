## Why

The project lacks consistent formatting, linting, and development workflows. Code quality tools, a task runner, and model auto-download will streamline development. A benchmark baseline is needed to prevent performance regressions during cleanup.

## What Changes

- Add `justfile` with targets: `format`, `lint`, `test`, `bench`, `baseline`, `dev`, `download-model`
- Add Go tooling: `gofmt`, `goimports`, `staticcheck`, `golangci-lint`
- Add `.gitignore` excluding `models/` and build artifacts
- Add config-driven model download (optional `model_url` field in config.yaml)
- Run all style tools across the codebase and apply fixes
- Generics refactoring where it improves code clarity or performance
- Establish benchmark baseline saved to `benchmark-baseline.txt`
- Performance validation: compare benchmark results before and after changes

## Capabilities

### New Capabilities
- `code-formatting`: Go formatting tools setup, `justfile` for development tasks, `.gitignore` configuration
- `model-autodownload`: Optional HuggingFace model URL in config, auto-downloaded on startup if not cached locally

### Modified Capabilities
- `model-loading`: Add `model_url` field to model config; if set and ONNX path doesn't exist, download and extract before loading

## Impact

Files affected: `justfile`, `.gitignore`, `config.yaml`, `internal/config/config.go`, `internal/registry/registry.go`, `cmd/emb/main.go`, `flake.nix`. No breaking changes to the RESP protocol or embedding pipeline. All existing tests must continue to pass with equal or better performance.
