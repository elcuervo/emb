## ADDED Requirements

### Requirement: CI runs on every push

The project SHALL run `just lint` and `just test` on every push to any branch.

#### Scenario: Push triggers CI

- **WHEN** a commit is pushed
- **THEN** GitHub Actions runs `just lint` and `just test`
- **THEN** the workflow reports pass/fail on the commit

### Requirement: Multi-arch Docker push on release

The project SHALL build and push a multi-arch Docker image to Docker Hub on every GitHub Release.

#### Scenario: Release triggers Docker push

- **WHEN** a GitHub Release is published
- **THEN** the image is built for `linux/amd64` and `linux/arm64`
- **THEN** the image is pushed to `elcuervo/emb-server:latest` and `elcuervo/emb-server:<tag>`

#### Scenario: Docker Hub credentials

- **WHEN** the release workflow runs
- **THEN** it authenticates to Docker Hub using `DOCKER_USERNAME` and `DOCKER_PASSWORD` secrets
