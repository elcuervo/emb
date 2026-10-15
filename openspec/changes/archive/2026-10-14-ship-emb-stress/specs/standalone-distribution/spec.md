# Spec Delta

## MODIFIED Requirements

### Requirement: Self-contained platform tarballs

Each supported platform SHALL have a self-contained tarball published as a GitHub Release asset containing the `emb` server binary, the `emb-top` dashboard binary, the `emb-stress` load generator, and the runtime dependency `libonnxruntime`.

#### Scenario: Linux amd64 tarball is self-contained

- **GIVEN** a Linux amd64 release tarball `emb_<version>_linux_amd64.tar.gz`
- **WHEN** extracted to an empty directory
- **THEN** the directory SHALL contain `emb`, `emb-top`, `emb-stress` and `libonnxruntime.so*`
- **AND** `./emb` SHALL run without any additional libraries installed

#### Scenario: macOS arm64 tarball is self-contained

- **GIVEN** a macOS arm64 release tarball `emb_<version>_darwin_arm64.tar.gz`
- **WHEN** extracted to an empty directory
- **THEN** the directory SHALL contain `emb`, `emb-top`, `emb-stress` and `libonnxruntime.dylib`
- **AND** `./emb` SHALL run without any additional libraries installed

#### Scenario: Linux arm64 tarball is self-contained

- **GIVEN** a Linux arm64 release tarball `emb_<version>_linux_arm64.tar.gz`
- **WHEN** extracted to an empty directory
- **THEN** the directory SHALL contain `emb`, `emb-top`, `emb-stress` and `libonnxruntime.so*`
- **AND** `./emb` SHALL run without any additional libraries installed

#### Scenario: The dashboard and the load generator need no runtime library

- **GIVEN** an extracted release tarball whose `libonnxruntime*` members have been removed
- **WHEN** `./emb-top -version` and `./emb-stress -version` are run
- **THEN** both SHALL print their version and exit 0

#### Scenario: The packaged load generator drives a node

- **GIVEN** an extracted release tarball and a node started from the same tarball
- **WHEN** `./emb-stress -config <config> -target <node address>` is run
- **THEN** it SHALL drive load against that node and report the run's levels

### Requirement: Install script

An install script SHALL exist at `install.sh` in the repository root. It SHALL:
- Detect the user's platform (OS + architecture)
- Resolve the latest GitHub release
- Download the matching tarball
- Extract `emb`, `emb-top`, `emb-stress` (and `libonnxruntime` where bundled) to the install directory
- Report the binaries it placed

#### Scenario: Install on macOS arm64

- **WHEN** the script runs on macOS arm64
- **THEN** it SHALL download `emb_<version>_darwin_arm64.tar.gz`
- **AND** place `emb`, `emb-top` and `emb-stress` in the install directory

#### Scenario: Install to custom directory

- **WHEN** the `EMB_INSTALL_DIR` environment variable is set
- **THEN** the script SHALL extract to `$EMB_INSTALL_DIR` instead of the default `/usr/local/bin`

#### Scenario: Unsupported platform

- **WHEN** the script runs on an unsupported platform (e.g., Windows, FreeBSD, Darwin x86_64)
- **THEN** it SHALL print an error and exit with code 1
