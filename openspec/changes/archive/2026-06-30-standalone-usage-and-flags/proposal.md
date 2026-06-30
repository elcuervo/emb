## Why

Running `emb` currently requires a YAML config file even for the simplest use case. A new user should be able to download the binary, point it at an ONNX model (or HuggingFace repo), and have it running in one command. This also makes the README quick-start example trivially executable.

## What Changes

- Add CLI flags for all common `Config` and `ModelConfig` fields so `emb` can run without a config file
- Support multiple models via flags: each `-model` flag starts a new model definition; subsequent flags apply to the last `-model` specified
- When no `-config` flag is given, skip YAML loading and use flag values alone
- Move `-config` from required to optional (with sensible defaults)
- Update README with a single-line standalone example that works with no config file, including a two-model test example using `EMB.MULTI`
- Add a `-version` flag to print the binary version

## Capabilities

### New Capabilities
- `cli-flags`: Command-line flags for server address, model path/repo, and model config (pooling, normalize, dim, etc.). `-config` becomes optional; when omitted, flags define models inline. Multiple models supported by repeating `-model` groups.

### Modified Capabilities
- (none — existing YAML config behavior unchanged)

## Impact

| File | Change |
|------|--------|
| `cmd/emb/main.go` | Add CLI flags, conditional config loading (YAML or flags), multi-model flag parsing |
| `internal/config/config.go` | Add `FromFlags()` or merge function to build `Config` from flags |
| `README.md` | Replace quick-start with a one-liner standalone example, add two-model test |
