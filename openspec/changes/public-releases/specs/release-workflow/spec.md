## ADDED Requirements

### Requirement: Single versions file

The project SHALL maintain a `.github/versions.env` file that defines the ONNX Runtime and libtokenizers versions used across all build paths (Dockerfile, justfile, CI workflows).

#### Scenario: justfile sources versions from versions.env

- **GIVEN** `.github/versions.env` defines `ORT_VERSION` and `TOKENIZERS_VERSION`
- **WHEN** `just build` is run
- **THEN** the libtokenizers download uses `TOKENIZERS_VERSION`

#### Scenario: Dockerfile receives versions via build args

- **GIVEN** `.github/versions.env` defines `ORT_VERSION`
- **WHEN** `docker build` is run
- **THEN** the Dockerfile uses the `ORT_VERSION` build arg (defaults from the file)

---

### Requirement: Smoke test after release build

Every release workflow SHALL run a smoke test on the compiled artifact before publishing.

#### Scenario: Linux artifact smoke test

- **GIVEN** the `release-linux` job has built the Docker image
- **WHEN** the smoke test step runs
- **THEN** it starts a container from the freshly-built image, runs `EMB.HELLO`, and verifies the response

#### Scenario: macOS artifact smoke test

- **GIVEN** the `release-macos` job has built the binary via goreleaser
- **WHEN** the smoke test step runs
- **THEN** it runs the binary against a pre-downloaded model and verifies `EMB.INFER` output shape

---

### Requirement: Release notes generation

The release workflow SHALL generate release notes that include a changelog and list of OpenSpec changes since the last release.

#### Scenario: Release notes include changelog

- **GIVEN** the release workflow is running
- **WHEN** the release is published
- **THEN** the release notes contain auto-generated changelog from git log

#### Scenario: Release notes include OpenSpec context

- **GIVEN** there are OpenSpec changes completed since the last release
- **WHEN** the release notes are generated
- **THEN** each completed OpenSpec change is listed with its description

## CHANGED Requirements

### Requirement: Docker Hub username parameterized (from docker-build spec)

Changed from hardcoded `elcuervo` to configurable via GitHub `vars.DOCKER_USER`.

#### Scenario: Docker push uses configurable user

- **GIVEN** the `release-linux` workflow is running
- **WHEN** it pushes to Docker Hub
- **THEN** it uses `${{ vars.DOCKER_USER }}` instead of a hardcoded string

---

### Requirement: Release triggered by version tag (from existing release.yml)

Changed from requiring a GitHub Release published event to also supporting direct tag pushes. This simplifies local automation (just `git tag` + `git push`).

#### Scenario: Release on tag push

- **GIVEN** a new semver tag is pushed (e.g., `v0.1.0`)
- **WHEN** the release workflow runs
- **THEN** it builds, smoke-tests, creates a GitHub Release (draft), and uploads artifacts

## REMOVED Requirements

### Requirement: Hardcoded ONNX Runtime arch `linux-x86_64`

The release workflow SHALL NOT use `linux-x86_64` as the ONNX Runtime asset name (it does not exist upstream). The correct name `linux-x64` SHALL be used. (Already fixed in this change.)
