## Why

The `emb` Ruby gem currently lives in `gem/` at the repo root, which doesn't scale as more gems are added. Moving it to `gems/emb/` makes room for future gems, gives each gem its own identity, and allows independent test workflows with `rake`. A README in the gem documents how to run the E2E test suite against a real `emb` server.

## What Changes

- Move `gem/` → `gems/emb/` keeping all internal paths intact
- Update `gems/emb/emb.gemspec` to reference `gems/emb/` paths
- Add `gems/emb/README.md` with E2E test instructions (start emb server, run `rake`)
- Add `gems/emb/Rakefile` with `require "rspec/core/rake_task"` so `rake` runs tests
- Add `gems/emb/.github/workflows/test.yml` for standalone CI (build emb binary, bundle install, run tests)
- Update workspace references (flake.nix, .gitignore if needed)

## Capabilities

### New Capabilities
- `gems-emb-ci`: Standalone CI for the `gems/emb/` Ruby gem with E2E testing against a real `emb` binary

### Modified Capabilities
- (none)

## Impact

| File | Change |
|------|--------|
| `gem/` → `gems/emb/` | **Moved** — entire gem directory |
| `gems/emb/Rakefile` | **Added** — `rake` runs `rspec` |
| `gems/emb/README.md` | **Added** — E2E test instructions |
| `gems/emb/.github/workflows/test.yml` | **Added** — standalone CI for the gem |
