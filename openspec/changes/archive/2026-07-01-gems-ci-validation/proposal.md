## Why

The gems are built, installed, and validated only via CI — there's no way to run the same validation locally before pushing. The `gems/emb/.github/workflows/test.yml` standalone CI duplicates the main release workflow's gem testing but has a different trigger pattern (only on `paths: ["gems/emb/**"]`). The `gems/emb-server/` gem has no standalone CI — it's only tested during release. Adding `act` (local GitHub Actions runner) enables running CI workflows locally before pushing.

## What Changes

- Add `act` to `flake.nix` devShell for local GitHub Actions testing
- Switch from `RUBYGEMS_API_KEY` to trusted publishing (OIDC) via `rubygems/configure-rubygems-credentials@main`, matching gte's release pattern
- Add post-install gem validation (`gem install --local` + smoke test) that works both locally and in CI
- Update `gems/emb/.github/workflows/test.yml` triggers: add `tags: ["v*"]` and `workflow_dispatch` for manual/local triggering
- Add `gems/emb-server/.github/workflows/test.yml` standalone CI that builds the gem and validates the binary
- Validate locally: `act` runs the full workflow, or `just validate-gems` runs the validation steps directly

## Capabilities

### New Capabilities
- (none)

### Modified Capabilities
- `emb-ruby-client`: Verified post-install via local and CI smoke test
- `emb-server-distribution`: Published via trusted publishing, standalone CI for build validation

## Impact

| File | Change |
|------|--------|
| `flake.nix` | Add `act` to devShell for local CI testing |
| `.github/workflows/release.yml` | Replace `RUBYGEMS_API_KEY` with OIDC trusted publishing, add `validate-gems` step after build, add `permissions` + `environment` to publish jobs |
| `gems/emb/.github/workflows/test.yml` | Add `tags: ["v*"]` and `workflow_dispatch` triggers |
| `gems/emb-server/.github/workflows/test.yml` | **Added** — standalone CI to build and validate the gem |
