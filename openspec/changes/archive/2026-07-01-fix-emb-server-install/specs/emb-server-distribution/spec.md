## MODIFIED Requirements

### Requirement: Thin bin wrapper

The `bin/emb` wrapper SHALL find the `onnxruntime` gem's shared library and exec the precompiled Go binary with `-ort-lib`.

#### Existing scenario replaced: ORT resolved at runtime

- **WHEN** `emb --model-repo Xenova/all-MiniLM-L6-v2` is run
- **THEN** the wrapper SHALL locate the onnxruntime library via `OnnxRuntime.ffi_lib.first`
- **THEN** it SHALL exec `emb-binary -ort-lib <path> --model-repo Xenova/all-MiniLM-L6-v2`
- **THEN** the server SHALL start and accept EMB commands

### Requirement: Portable binary (new)

The Go binary SHALL be compiled without a compile-time link to onnxruntime, so it can start on any machine regardless of where ORT is installed.

#### Scenario: Binary has no dyld dependency on ORT

- **GIVEN** the binary is built with `CGO_ENABLED=1`
- **WHEN** `otool -L emb-binary-arm64-darwin` is run
- **THEN** no `libonnxruntime` entry SHALL appear in the output

#### Scenario: Binary starts without ORT installed

- **GIVEN** the binary exists on a machine without onnxruntime
- **WHEN** `./emb -version` is run
- **THEN** the version SHALL print without error

#### Scenario: ORT loaded via -ort-lib at runtime

- **GIVEN** the binary is running with `-ort-lib <path-to-libonnxruntime.dylib>`
- **WHEN** the server receives an `EMB` command
- **THEN** it SHALL successfully run inference
