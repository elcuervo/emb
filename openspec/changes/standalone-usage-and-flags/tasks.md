## 1. Config Parsing from Flags

- [x] 1.1 Add `ParseFlags()` function to `config.go` that reads args and returns a `FlagConfig`: handles `-config`, `-listen`, `-version`, `-ort-lib`, and model-level flags grouped by `-model` occurrences
- [x] 1.2 Handle single-model shorthand (no `-model` → default key `"model"`) and multi-model via repeated `-model`
- [x] 1.3 Validate: require at least one model has `ONNX` or `ModelRepo` set (skip validation when `-config` is used)

## 2. CLI Flags in main.go

- [x] 2.1 Add all flags via `config.ParseFlags()` (server-level: `-listen`, `-config`, `-version`, `-ort-lib`; model-level: `-model`, `-model-onnx`, `-model-repo`, `-model-tokenizer`, `-pooling`, `-normalize`, `-dim`, `-max-length`, `-output-tensor`, `-pad-output`, `-workers`, `-intra-op-threads`, `-inter-op-threads`)
- [x] 2.2 Add `-version` flag with `var version = "dev"` ldflag support
- [x] 2.3 Replace hardcoded config loading with single `ParseFlags()` call

## 3. README Standalone Example

- [x] 3.1 Add a single-model one-liner example: `emb -model-repo Xenova/all-MiniLM-L6-v2`
- [x] 3.2 Add a two-model inline example showing `EMB.MULTI` testing: `emb -model minilm -model-onnx ./minilm.onnx -model bge -model-repo Xenova/bge-small-en-v1.5`
- [x] 3.3 Keep existing quick-start section for local development workflow

## 4. Build Wiring

- [x] 4.1 Update `justfile` with `-ldflags` for version embedding in `build` recipe
- [x] 4.2 Update `.goreleaser.yaml` to inject version via ldflags
