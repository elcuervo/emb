## ADDED Requirements

### Requirement: Clean job naming

The release workflow SHALL use job names that describe what each job produces:

| Job | Produces | Depends on |
|-----|----------|------------|
| `test` | Test results | — |
| `build-linux-amd64` | `emb_*_linux_amd64.tar.gz` | `test` |
| `build-linux-arm64` | `emb_*_linux_arm64.tar.gz` | `test` |
| `build-darwin-arm64` | `emb_*_darwin_arm64.tar.gz` | `test` |
| `docker` | Docker multi-arch manifest `linux/amd64 + linux/arm64` | all `build-*` |

#### Scenario: Pipeline runs in order

- **WHEN** a release is triggered
- **THEN** all `build-*` jobs run in parallel after `test` completes
- **THEN** `docker` runs after all `build-*` jobs complete

### Requirement: Three binary artifacts

The release SHALL produce three binary tarballs:

- `emb_{version}_linux_amd64.tar.gz` (Go binary + `libonnxruntime.so*`)
- `emb_{version}_linux_arm64.tar.gz` (Go binary + `libonnxruntime.so*`)
- `emb_{version}_darwin_arm64.tar.gz` (Go binary + `libonnxruntime*.dylib`)

Each tarball SHALL be uploaded to the GitHub Release.

#### Scenario: All three tarballs attached to release

- **WHEN** a release workflow completes
- **THEN** the GitHub Release contains all three tarballs and a `checksums.txt`

### Requirement: Multi-arch Docker image pushed

The release SHALL push a single multi-arch Docker manifest covering `linux/amd64` and `linux/arm64`, tagged as `{version}` and `latest`.

#### Scenario: Single docker push

- **WHEN** `docker` job runs
- **THEN** it builds and pushes with `--platform linux/amd64,linux/arm64 --push`
- **THEN** the Docker Hub has `elcuervo/emb:{version}` and `elcuervo/emb:latest`
