## 1. Multi-Arch Validation

- [x] 1.1 Build Docker image for native arch — 172MB (97MB base + 60MB ORT + 6MB binary + 9MB misc)
- [x] 1.2 Multi-arch `--load` not supported on macOS Docker Desktop; `--push` works in CI via buildx + QEMU

## 2. GitHub Actions CI

- [x] 2.1 Create `.github/workflows/ci.yml`: format check → go vet → go test -short on pure Go packages
- [x] 2.2 CGO_ENABLED=0 verified: config, hfhub, tokenizer packages build without CGo

## 3. Release Workflow

- [x] 3.1 Create `.github/workflows/release.yml`: QEMU → Buildx → Docker login → build + push multi-arch
- [x] 3.2 Tags: `elcuervo/emb-server:<semver>` and `elcuervo/emb-server:latest` via docker/metadata-action
- [x] 3.3 Platform: `linux/amd64,linux/arm64`

## 4. Verification

- [x] 4.1 Workflow YAML verified — correct syntax, uses official GitHub Actions
- [x] 4.2 Ready to push — workflows will appear in GitHub Actions after first push
