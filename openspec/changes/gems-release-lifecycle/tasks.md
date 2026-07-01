## 1. Add gem testing to release workflow

- [x] 1.1 Add `test-gems` job to `.github/workflows/release.yml` that downloads the `emb` binary from the release, starts the server, runs `gems/emb` RSpec suite, and validates `gems/emb-server` gem build
- [x] 1.2 Wire dependencies: `test-gems` depends on all `build-*` jobs; `release-emb` and `release-emb-server` depend on `test-gems`

## 2. Update emb-server gem for multi-arch

- [x] 2.1 Update `gems/emb-server/bin/emb` to select the platform-specific binary at runtime via `RUBY_PLATFORM` case
- [x] 2.2 Remove platform-specific gemspec (`spec.platform`) — single gem ships all binaries
- [x] 2.3 Update `gems/emb-server/lib/emb-server/` with version.rb and all 3 platform binaries

## 3. Publish both gems

- [x] 3.1 Add `release-emb` job: builds and pushes `gems/emb` (pure Ruby, platform-independent)
- [x] 3.2 Add `release-emb-server` job: downloads all 3 binaries from release tarballs, copies into gem structure, builds and pushes single multi-arch gem
