## Why

`gems/emb` (the Ruby client) and `gems/emb-server` (the distribution gem) exist but aren't fully integrated into the release lifecycle. The Ruby client has no release pipeline — it's never published. The distribution gem's release job exists but doesn't test the client gem as part of the pipeline. A release should validate both gems and publish them together with the same version.

## What Changes

- Add a `test-gems` job to the release workflow that starts the `emb` server and runs `gems/emb` RSpec suite and validates `gems/emb-server` gem build
- Add a `release-gem` step to build and push `gems/emb` (pure Ruby, no platform variant) alongside `gems/emb-server`
- Update `gems/emb` to be publishable: gemspec doesn't hardcode files list (or includes all needed files)
- Ensure `gems/emb/.github/workflows/test.yml` stays in sync with the main workflow's gem testing

## Capabilities

### New Capabilities
- (none)

### Modified Capabilities
- `emb-ruby-client`: Now published to rubygems.org as part of the release pipeline
- `emb-server-distribution`: Release order ensures gems are tested before publishing

## Impact

| File | Change |
|------|--------|
| `.github/workflows/release.yml` | Add `test-gems` job (starts server, runs RSpec + gem build validation). Update `release-gem` to include `gems/emb` push. Add dependency chain. |
