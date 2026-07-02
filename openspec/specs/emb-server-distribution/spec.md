# emb-server-distribution

## Purpose

Specifies how the `emb-server` Ruby gem packages and distributes the precompiled `emb` Go binary across multiple platforms.

## Requirements

### Requirement: Multi-arch gem distribution

The `emb-server` gem SHALL distribute precompiled `emb` binaries for multiple platforms. The gem platform SHALL be auto-detected at build time from `RUBY_PLATFORM`.

#### Scenario: Install on Apple Silicon

- **WHEN** `gem install emb-server` is run on an `arm64-darwin` system
- **THEN** the `arm64-darwin` gem variant SHALL be selected
- **THEN** `emb` SHALL be available on PATH

#### Scenario: Install on Linux AMD64

- **WHEN** `gem install emb-server` is run on an `x86_64-linux` system
- **THEN** the `x86_64-linux` gem variant SHALL be selected

### Requirement: Thin bin wrapper

The `bin/emb` wrapper SHALL find the `onnxruntime` gem's shared library and exec the precompiled Go binary with `-ort-lib`.

#### Scenario: ORT lib resolved via gem API

- **WHEN** `emb --model-repo Xenova/all-MiniLM-L6-v2` is run
- **THEN** the wrapper SHALL require `onnxruntime`
- **THEN** the wrapper SHALL use `OnnxRuntime.ffi_lib.first` to locate the library
- **THEN** it SHALL exec `emb-binary -ort-lib <path> --model-repo Xenova/all-MiniLM-L6-v2`
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

### Requirement: Portable binary

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

### Requirement: onnxruntime gem dependency

The gemspec SHALL declare `onnxruntime ~> 0.11` as a runtime dependency, ensuring ORT 1.27.0+ is bundled.

#### Scenario: Dependency installed automatically

- **WHEN** `gem install emb-server` completes
- **THEN** the `onnxruntime` gem SHALL also be installed

#### Scenario: ORT runtime version compatibility

- **GIVEN** the `emb-server` gem is installed in a project
- **WHEN** `bundle exec emb -model-repo Xenova/all-MiniLM-L6-v2` is run
- **THEN** the Gemfile.lock SHALL resolve to `onnxruntime ~> 0.11`
- **THEN** the bundled ORT SHALL be version 1.27.0 or newer
- **THEN** the server SHALL start and accept EMB commands

### Requirement: Release pipeline publishes platform gems

The release workflow SHALL include a job that builds platform-specific gems and publishes them to rubygems.org.

#### Scenario: Gem built from CI binary

- **WHEN** the release workflow runs
- **THEN** the `emb-binary` from each platform build SHALL be copied into `gems/emb-server/lib/emb-server/`
- **THEN** `gem build emb-server.gemspec` SHALL produce the platform-specific `.gem` file
- **THEN** `gem push` SHALL publish it to rubygems.org
