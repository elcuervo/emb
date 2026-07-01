## Why

The `test-gems` job in the release workflow failed during the `v0.1.0-test` run. Two issues were found:

1. `test-two-models.yaml` is gitignored by the `*.yaml` pattern, so the CI checkout doesn't include it. The server can't start without this config.
2. The `test-gems`, `release-emb`, and `release-emb-server` jobs run Ruby/bundler/gem commands but don't install Ruby via `ruby/setup-ruby@v1`. The `ubuntu-latest` runner doesn't include `bundler` by default, and `gem` availability is not guaranteed.

## How

1. Remove `*.yaml` from `.gitignore` so `test-two-models.yaml` (and future yaml configs needed by CI) can be tracked.
2. Add `ruby/setup-ruby@v1` step to `test-gems`, `release-emb`, and `release-emb-server` jobs in `release.yml`.

## What

| File | Change |
|------|--------|
| `.gitignore` | Remove `*.yaml` line |
| `.github/workflows/release.yml` | Add `ruby/setup-ruby@v1` to `test-gems`, `release-emb`, `release-emb-server` jobs |
