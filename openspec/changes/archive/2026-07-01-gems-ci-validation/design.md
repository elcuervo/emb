## Context

Three validation layers exist: the Go test suite, the Ruby client gem tests, and the distribution gem build. The gem CIs are not aligned with the release workflow triggers, and there's no way to validate locally before pushing. `act` provides local GitHub Actions simulation.

## Goals / Non-Goals

**Goals:**
- Full local validation: build → install → smoke test both gems
- `act run` executes workflows locally (requires Docker, runs on Linux)
- Both gem CIs trigger on tags and support `workflow_dispatch`
- `emb-server` gets its own CI workflow for build validation
- Trusted publishing (OIDC) for gem release, matching gte

**Non-Goals:**
- Cross-platform local validation (`act` on macOS can't run Linux containers natively)
- Full E2E server test in gem CIs (they validate the gem packaging, not the server)

## Decisions

### Decision: Local validation via `just validate-gems`

A new recipe that builds, installs, and smokes both gems locally:

```bash
just validate-gems
  → cd gems/emb && gem build emb.gemspec
  → gem install --local gems/emb/emb-*.gem
  → ruby -e "require 'emb'; puts Emb::VERSION"    # validates pure Ruby gem
  → cd gems/emb-server && cp ../emb/../../bin/emb lib/emb-server/emb-binary-arm64-darwin && gem build emb-server.gemspec
  → gem install --local gems/emb-server/emb-server-*.gem
  → which emb                                         # validates binary on PATH
  → emb -version                                      # validates version flag
```

For `emb-server`, the `onnxruntime` gem dependency must be installed first. If missing, the install fails with a clear error.

### Decision: `act` for local CI simulation

`act` is added to the nix devShell. Usage:

```bash
# Run the full release workflow locally (simulates tag push)
act push --eventpath <(echo '{"ref":"refs/tags/v0.1.0"}')

# Run just the gem CI
act --workflows gems/emb/.github/workflows/test.yml
```

Note: `act` requires Docker. On macOS, Docker Desktop or Colima must be running. The `act` binary is provided by nix but Docker is a system dependency.

### Decision: Gem CI trigger alignment

**`gems/emb/.github/workflows/test.yml`** (standalone gem CI):
```yaml
on:
  push:
    branches: ["main"]
    tags: ["v*"]
    paths: ["gems/emb/**"]
  pull_request:
    paths: ["gems/emb/**"]
  workflow_dispatch:
```

This runs on:
- PRs touching gem code
- Pushes to main touching gem code
- Tag pushes (any, not just gem paths — because tags are releases)
- Manual trigger via `workflow_dispatch`

**`gems/emb-server/.github/workflows/test.yml`** (new):
```yaml
on:
  push:
    branches: ["main"]
    tags: ["v*"]
    paths: ["gems/emb-server/**"]
  pull_request:
    paths: ["gems/emb-server/**"]
  workflow_dispatch:
```

Validates the gem can be built from source (copies a binary placeholder, builds gem, validates structure).

### Decision: Pipeline with local and CI paths

```
Local:  nix develop → just validate-gems
        nix develop → act run --job test-gems

CI:     gem workflow (tag push or PR)
        main release workflow (tag push)
```

Both paths validate the same thing: gem builds, installs, and runs.

## Risks / Trade-offs

- **`act` doesn't simulate GitHub secrets** — trusted publishing (OIDC) can't be tested locally with `act`. The `gem push` step would fail. Mitigation: validate up to `gem build` locally, push only in CI.
- **`act` on macOS needs Docker** — not all developers have Docker running. Mitigation: `just validate-gems` doesn't need Docker at all.
- **`emb-server` CI needs a binary** — the CI copies the binary from a `just build` or a downloaded release artifact. For `workflow_dispatch`, it builds from source.
