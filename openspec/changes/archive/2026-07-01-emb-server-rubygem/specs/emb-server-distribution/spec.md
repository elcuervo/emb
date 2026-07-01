## ADDED Requirements

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

#### Scenario: ORT lib resolved at runtime

- **WHEN** `emb --model-repo Xenova/all-MiniLM-L6-v2` is run
- **THEN** the wrapper SHALL locate `onnxruntime.{dylib,so}` via `Gem::Specification.find_by_name("onnxruntime")`
- **THEN** it SHALL exec `emb-binary -ort-lib <path> --model-repo Xenova/all-MiniLM-L6-v2`
- **THEN** the server SHALL start and accept EMB commands

### Requirement: onnxruntime gem dependency

The gemspec SHALL declare `onnxruntime` as a runtime dependency.

#### Scenario: Dependency installed automatically

- **WHEN** `gem install emb-server` completes
- **THEN** the `onnxruntime` gem SHALL also be installed

### Requirement: Release pipeline publishes platform gems

The release workflow SHALL include a job that builds platform-specific gems and publishes them to rubygems.org.

#### Scenario: Gem built from CI binary

- **WHEN** the release workflow runs
- **THEN** the `emb-binary` from each platform build SHALL be copied into `gems/emb-server/lib/emb-server/`
- **THEN** `gem build emb-server.gemspec` SHALL produce the platform-specific `.gem` file
- **THEN** `gem push` SHALL publish it to rubygems.org
