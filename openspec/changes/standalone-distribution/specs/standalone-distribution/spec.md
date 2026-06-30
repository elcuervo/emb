## ADDED Requirements

### Requirement: Self-contained platform tarballs

Each supported platform SHALL have a self-contained tarball published as a GitHub Release asset containing both the `emb` binary and its runtime dependency `libonnxruntime`.

#### Scenario: Linux amd64 tarball is self-contained

- **GIVEN** a Linux amd64 release tarball `emb_<version>_linux_amd64.tar.gz`
- **WHEN** extracted to an empty directory
- **THEN** the directory SHALL contain both `emb` and `libonnxruntime.so*`
- **AND** `./emb` SHALL run without any additional libraries installed

#### Scenario: macOS arm64 tarball is self-contained

- **GIVEN** a macOS arm64 release tarball `emb_<version>_darwin_arm64.tar.gz`
- **WHEN** extracted to an empty directory
- **THEN** the directory SHALL contain both `emb` and `libonnxruntime.dylib`
- **AND** `./emb` SHALL run without any additional libraries installed

#### Scenario: Linux arm64 tarball is self-contained

- **GIVEN** a Linux arm64 release tarball `emb_<version>_linux_arm64.tar.gz`
- **WHEN** extracted to an empty directory
- **THEN** the directory SHALL contain both `emb` and `libonnxruntime.so*`
- **AND** `./emb` SHALL run without any additional libraries installed

### Requirement: Unified naming convention

All release tarballs SHALL follow the naming convention `emb_<version>_<os>_<arch>.tar.gz`.

#### Scenario: Tarball naming

- **GIVEN** a release tagged `v0.1.0`
- **WHEN** the release workflow runs for all three platforms
- **THEN** the following assets SHALL exist:
  - `emb_0.1.0_linux_amd64.tar.gz`
  - `emb_0.1.0_linux_arm64.tar.gz`
  - `emb_0.1.0_darwin_arm64.tar.gz`

### Requirement: Install script

An install script SHALL exist at `install.sh` in the repository root. It SHALL:
- Detect the user's platform (OS + architecture)
- Resolve the latest GitHub release
- Download the matching tarball
- Extract `emb` (and `libonnxruntime` where bundled) to the install directory

#### Scenario: Install on macOS arm64

- **WHEN** the script runs on macOS arm64
- **THEN** it SHALL download `emb_<version>_darwin_arm64.tar.gz`
- **AND** place `emb` in the install directory

#### Scenario: Install to custom directory

- **WHEN** the `EMB_INSTALL_DIR` environment variable is set
- **THEN** the script SHALL extract to `$EMB_INSTALL_DIR` instead of the default `/usr/local/bin`

#### Scenario: Unsupported platform

- **WHEN** the script runs on an unsupported platform (e.g., Windows, FreeBSD, Darwin x86_64)
- **THEN** it SHALL print an error and exit with code 1

### Requirement: macOS binary uses @loader_path

The macOS binary SHALL use `@loader_path` to find `libonnxruntime.dylib` so it resolves relative to the binary's own directory.

#### Scenario: Binary finds ORT when co-located

- **GIVEN** a macOS release binary
- **WHEN** `otool -L emb` is run
- **THEN** the `libonnxruntime.dylib` entry SHALL reference `@loader_path/libonnxruntime.dylib`
