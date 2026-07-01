## Why

The project currently has three separate version sources: `git describe` for the Go binary, `gems/emb/VERSION`, and `gems/emb-server/VERSION`. This drift risk is real — a gem could be published with a version that doesn't match the Go binary it contains. A single root `VERSION` file keeps everything in sync.

## What Changes

- Add `VERSION` at repo root containing `0.1.0`
- `gems/emb/VERSION` reads from `../../VERSION` (exit with error if root file missing)
- `gems/emb-server/VERSION` reads from `../../VERSION`
- Justfile reads root `VERSION` instead of `git describe` for the version ldflag
- Release CI: before building gems, overwrite root `VERSION` with the release tag on tag pushes
- All builds now produce artifacts tagged with the same version

## Capabilities

### New Capabilities
- `project-versioning`: Single root `VERSION` file as source of truth for Go binary, Ruby gems, and CI artifacts

### Modified Capabilities
- (none)

## Impact

| File | Change |
|------|--------|
| `VERSION` | **Added** at repo root — `0.1.0` |
| `gems/emb/VERSION` | Change from hardcoded to reading `../../VERSION` |
| `gems/emb-server/VERSION` | Change from hardcoded to reading `../../VERSION` |
| `gems/emb/emb.gemspec` | Update path to root VERSION |
| `gems/emb-server/emb-server.gemspec` | Update path to root VERSION |
| `justfile` | `image_tag` reads `VERSION` file instead of `git describe` |
| `.github/workflows/release.yml` | On tag push, write tag version to root VERSION before builds execute |
