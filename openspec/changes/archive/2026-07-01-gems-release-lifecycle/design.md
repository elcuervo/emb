## Context

The project has three release artifacts: the `emb` Go binary (via tarballs and Docker), the `emb` Ruby client gem (`gems/emb`), and the `emb-server` distribution gem (`gems/emb-server`). Only the Go binary and `emb-server` gem are published during a release. The Ruby client gem is not published, and neither gem is tested as part of the release pipeline.

## Goals / Non-Goals

**Goals:**
- Release workflow tests both gems before publishing
- `gems/emb` published to rubygems.org on release (platform-independent `ruby` gem)
- `gems/emb-server` published as a single multi-arch gem shipping all platform binaries
- Test order: Go server → gems (which depend on the server) → publish gems
- CI workflow for `gems/emb` stays consistent with main release testing

**Non-Goals:**
- Publishing to rubygems.org on push (only on release/tag)
- Separate release workflows per gem (single orchestrated flow)
- `gems/emb-server` RSpec tests (it's a distribution-only gem)

## Decisions

### Decision: Pipeline with gem testing stage

```
test (Go)
  ├─ build-linux-amd64 ─────────────────┐
  ├─ build-linux-arm64 ─────────────────┤
  └─ build-darwin-arm64 ────────────────┤
                                        ▼
                                 test-gems
                                        │
                         ┌──────────────┼──────────────┐
                         ▼              ▼              ▼
                     release-emb    release-emb-server  docker
                     (ruby gem)     (multi-arch gem)
```

`test-gems` runs after all `build-*` jobs complete. It:
1. Downloads the `emb` binary from GitHub Release artifacts (one platform, e.g., `linux-amd64`)
2. Starts the `emb` server
3. Runs `gems/emb` RSpec suite
4. Validates `gems/emb-server` gem can build
5. Stops the server

After `test-gems` passes, gem release jobs run in parallel with `docker`.

### Decision: Single multi-arch emb-server gem

`gems/emb-server` ships all three platform binaries inside a single gem. The `bin/emb` wrapper selects the right one at runtime:

```
lib/emb-server/
  emb-binary-arm64-darwin
  emb-binary-x86_64-linux
  emb-binary-aarch64-linux
  version.rb
bin/
  emb
```

Runtime selection in `bin/emb`:

```ruby
binary_name = case RUBY_PLATFORM
when /arm64.*darwin/  then "emb-binary-arm64-darwin"
when /x86_64.*linux/  then "emb-binary-x86_64-linux"
when /aarch64.*linux/ then "emb-binary-aarch64-linux"
else raise "unsupported platform: #{RUBY_PLATFORM}"
end
binary = File.join(spec.gem_dir, "lib", "emb-server", binary_name)
```

This avoids per-platform gemspec, multiple `gem push` commands, and simplifies the release CI. The tradeoff is a larger gem download (~60MB vs ~20MB).

### Decision: Gem release job

Two release jobs:

```yaml
release-emb:
  needs: [test-gems]
  steps:
    - cd gems/emb && gem build emb.gemspec
    - gem push emb-*.gem

release-emb-server:
  needs: [test-gems]
  steps:
    - Download all 3 binaries from release tarballs
    - Copy each into gems/emb-server/lib/emb-server/
    - cd gems/emb-server && gem build emb-server.gemspec
    - gem push emb-server-*.gem
```

## Risks / Trade-offs

- **Gem publishing order** — `emb` gem doesn't depend on `emb-server` and vice versa, so they can publish in parallel. No ordering constraint between them.
- **Binary needed for test-gems** — `test-gems` needs the `emb` binary to start the server. It downloads from the release artifacts (same approach as `release-gem`).
- **Credentials** — `RUBYGEMS_API_KEY` secret needed for both gems. Already exists for `emb-server`, works for `emb` too since it's the same rubygems.org account.
