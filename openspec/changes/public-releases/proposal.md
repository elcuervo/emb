## Why

The current release process has several flaws exposed by the 0.0.1 release attempt:

1. **Fragile dependency URLs** — The ONNX Runtime asset name uses `linux-x86_64` but upstream ships `linux-x64`. This silently broke the entire release pipeline.
2. **No artifact validation** — The workflow builds binaries and Docker images but never runs a smoke test to verify they work.
3. **No release notes** — GitHub releases contain bare tarballs with no changelog, no known-issues, no verification instructions.
4. **Split tooling** — macOS uses GoReleaser; Linux uses manual Docker extraction. Two paths, different config, different metadata.
5. **No version automation** — Tags are created manually. No `CHANGELOG.md`. No `--version` flag in the binary.
6. **Hardcoded Docker Hub user** — `elcuervo` is hardcoded in the workflow and justfile instead of a configurable variable.

A clean public release pipeline should be resilient, verifiable, and consistent across platforms.

## What Changes

- Unify Linux and macOS releases under GoReleaser (use Docker-based goreleaser for Linux cross-compilation, or extract binaries from Docker for Linux)
- Add a **smoke test** step that:
  - Starts the released binary
  - Downloads a small model via HF hub
  - Runs an EMB.HELLO and EMB.INFER
  - Verifies output shape and normalization
- Automate release notes from commit log + OpenSpec change summaries
- Replace hardcoded `elcuervo` Docker user with `${{ vars.DOCKER_USER }}`
- Fix the ONNX Runtime download URL (already done)
- Standardize dependency versions in a single source of truth (a `.github/versions.env` or similar)

## Capabilities

### New Capabilities
- `release-validation`: Automated smoke test for released artifacts

### Modified Capabilities
- `ci`: Test job runs on pushes/PRs
- `docker-build`: Multi-arch Docker build (unchanged interface, cleaner impl)

## Impact

| File | Change |
|------|--------|
| `.github/workflows/release.yml` | Add smoke test, use goreleaser for linux, fix URL, add release notes |
| `.goreleaser.yaml` | Add linux builds, add metadata, add changelog config |
| `justfile` | Add `release-smoke-test` recipe, parameterize Docker user |
| `.github/versions.env` | **Added** — single source for ORT/tokenizers versions |
| `Dockerfile` | Minor: parameterize versions from build args |

All existing capabilities unchanged. No breaking changes to user-facing interfaces.
