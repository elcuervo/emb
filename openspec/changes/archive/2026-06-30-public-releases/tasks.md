## Task List

### 1. Create `.github/versions.env`

- [x] Create `.github/versions.env` with `ORT_VERSION=v1.27.0` and `TOKENIZERS_VERSION=v1.27.0`
- [x] Update `release.yml` to source `.github/versions.env` (remove env block)
- [x] Update `justfile` to source `.github/versions.env` (replace hardcoded `libtokenizers-version`)
- [x] Update `Dockerfile` to accept `ORT_VERSION` and `TOKENIZERS_VERSION` as build args, defaulting from `versions.env`

### 2. Fix ONNX Runtime download URL

- [x] Change `linux-x86_64` to `linux-x64` in `release.yml` line 29

### 3. Add smoke test to release workflow

- [x] Add `smoke-test-linux` job (after `release-linux`) that:
  - Pulls the freshly-built Docker image
  - Starts a container with a test config
  - Runs PING and EMB.HELP to verify server responds
- [x] Add `smoke-test-macos` job (after `release-macos`) that:
  - Downloads a test model
  - Builds binary from source
  - Starts the server and runs PING + EMB.MODELS smoke test

### 4. Generate release notes

- [x] Add release notes generation via `softprops/action-gh-release` with `generate_release_notes: true`

### 5. Parameterize Docker Hub user

- [x] Replace `elcuervo` in `release.yml` with `${{ vars.DOCKER_USER }}`
- [x] Replace `elcuervo` in `justfile` with a variable (`docker_user := "elcuervo"`)
- [x] `.goreleaser.yaml` only has `elcuervo` as GitHub owner (not Docker Hub) — kept as-is

### 6. Add tag-based trigger

- [x] Add `push: tags: ["v*"]` to `release.yml` triggers (in addition to `release: [published]`)
- [x] Conditional: `softprops/action-gh-release` creates draft release on tag push, uploads to existing on release publish
