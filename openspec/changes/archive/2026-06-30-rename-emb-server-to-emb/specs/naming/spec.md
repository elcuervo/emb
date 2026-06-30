## MODIFIED Requirements

### Requirement: Docker push to elcuervo/emb

The Docker image SHALL be published as `elcuervo/emb` instead of `elcuervo/emb-server`.

#### Scenario: Docker image naming

- **WHEN** a Docker image is built for release
- **THEN** the image SHALL be tagged as `elcuervo/emb:latest` and `elcuervo/emb:<version>`
- **THEN** the image SHALL support linux/amd64 and linux/arm64
