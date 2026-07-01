## MODIFIED Requirements

### Requirement: Thin bin wrapper

The `bin/emb` wrapper SHALL find the `onnxruntime` gem's shared library and exec the precompiled Go binary with `-ort-lib`.

#### Scenario: ORT lib resolved via gem API

- **WHEN** `emb` is run
- **THEN** the wrapper SHALL require `onnxruntime`
- **THEN** the wrapper SHALL use `OnnxRuntime.ffi_lib.first` to locate the library
- **THEN** it SHALL exec `emb-binary -ort-lib <path> --model-repo ...`
- **THEN** the server SHALL start and accept EMB commands

#### Scenario: ORT lib resolved on Apple Silicon

- **GIVEN** the `onnxruntime` gem is installed on `arm64-darwin`
- **WHEN** `emb` is run
- **THEN** `OnnxRuntime.ffi_lib` SHALL return `vendor/libonnxruntime.arm64.dylib`
- **THEN** the wrapper SHALL pass it as `-ort-lib`

#### Scenario: ORT lib resolved on Linux AMD64

- **GIVEN** the `onnxruntime` gem is installed on `x86_64-linux`
- **WHEN** `emb` is run
- **THEN** `OnnxRuntime.ffi_lib` SHALL return `vendor/libonnxruntime.so`
- **THEN** the wrapper SHALL pass it as `-ort-lib`
