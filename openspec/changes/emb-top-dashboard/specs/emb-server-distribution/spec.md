## ADDED Requirements

### Requirement: emb-server gem distributes emb-top

The `emb-server` gem SHALL distribute a precompiled `emb-top` binary alongside the `emb` binary, one per supported platform, with a `bin/emb-top` wrapper that execs the platform binary. It SHALL NOT require the `onnxruntime` gem (the dashboard is a pure RESP client).

#### Scenario: Install on Apple Silicon

- **WHEN** `gem install emb-server` is run on an `arm64-darwin` system
- **THEN** the `arm64-darwin` gem variant SHALL be selected
- **THEN** `emb-top` SHALL be available on PATH and connect to a running node without the `onnxruntime` gem

#### Scenario: Install on Linux AMD64

- **WHEN** `gem install emb-server` is run on an `x86_64-linux` system
- **THEN** the `x86_64-linux` gem variant SHALL be selected
- **THEN** `emb-top` SHALL be available on PATH

#### Scenario: Thin wrapper

- **WHEN** `emb-top` is resolved from the gem
- **THEN** the wrapper SHALL locate `emb-top-binary-<platform>` inside the gem directory and exec it with the given arguments