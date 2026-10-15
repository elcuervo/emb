# Spec Delta

## ADDED Requirements

### Requirement: emb-server gem distributes emb-stress

The `emb-server` gem SHALL distribute a precompiled `emb-stress` binary alongside `emb` and `emb-top`, one per supported platform, with a `bin/emb-stress` wrapper that execs the platform binary with the given arguments. It SHALL NOT require the `onnxruntime` gem.

#### Scenario: Install on Apple Silicon

- **WHEN** `gem install emb-server` is run on an `arm64-darwin` system
- **THEN** the `arm64-darwin` gem variant SHALL be selected
- **THEN** `emb-stress` SHALL be available on PATH and drive a running node without the `onnxruntime` gem

#### Scenario: Install on Linux AMD64

- **WHEN** `gem install emb-server` is run on an `x86_64-linux` system
- **THEN** the `x86_64-linux` gem variant SHALL be selected
- **THEN** `emb-stress` SHALL be available on PATH

#### Scenario: Thin wrapper

- **WHEN** `emb-stress` is resolved from the gem
- **THEN** the wrapper SHALL locate `emb-stress-binary-<platform>` inside the gem directory and exec it with the given arguments
