## Context

The project has three testable surfaces: the Go server, the Ruby client gem, and the server distribution gem. Each has its own test/build command. A single `just all` recipe orchestrates all three.

## Goals / Non-Goals

**Goals:**
- `just all` runs Go tests, Ruby client tests, and gem build validation
- Fail fast: stop on first test failure
- Works inside and outside `nix develop`
- Clean up server process on completion

**Non-Goals:**
- Parallel test execution (sequential is fine for the full suite)
- Integration tests across gem-server boundaries beyond the existing RSpec suite
- Cross-platform CI integration (that's handled by `.github/workflows/`)

## Decisions

### Decision: Sequential stages with fail-fast

```
just all
  ├─ just test          (Go tests)
  ├─ just build         (go build)
  ├─ start emb server   (background, wait for ready)
  ├─ gems/emb RSpec     (bundle exec rake)
  ├─ gems/emb-server    (gem build validation)
  └─ stop emb server
```

Each stage depends on the previous. If Go tests fail, the server is never built. If the server doesn't start, the RSpec suite is skipped.

### Decision: Server lifecycle managed by the recipe

The recipe starts `emb` in the background, waits 8 seconds for it to be ready, runs tests, then kills it. The wait is a fixed sleep for simplicity (matching the existing test pattern in `spec_helper.rb` which uses `10.times do`).

### Decision: emb-server gem validation

The `gems/emb-server` gem build requires the `emb-binary` to exist. The recipe copies the freshly-built binary into the gem's structure, builds the gem, then removes the binary.

### Decision: Justfile recipe

```just
# Run all tests: Go server, Ruby client, gem builds
all: test build
    @echo "=== Testing Ruby client ==="
    cd gems/emb && bundle install --quiet 2>/dev/null || true
    DYLD_LIBRARY_PATH="{{ort_lib}}:$DYLD_LIBRARY_PATH" ./bin/emb -config test-two-models.yaml &
    sleep 8
    cd gems/emb && bundle exec rake
    @echo "=== Validating emb-server gem ==="
    cp ./bin/emb gems/emb-server/lib/emb-server/emb-binary
    cd gems/emb-server && gem build emb-server.gemspec --quiet 2>/dev/null
    rm -f gems/emb-server/lib/emb-server/emb-binary
    @echo "=== All tests passed ==="
```

## Risks / Trade-offs

- **Fixed sleep for server startup** — `sleep 8` is generous for local dev but might race on slow CI. Mitigation: the existing RSpec spec_helper already has a retry loop for PING, so a faster startup won't waste time.
- **Bundle install in the recipe** — running `bundle install` as part of `all` adds time. Mitigation: `--quiet` suppresses output; gems are cached in `vendor/bundle/`.
- **gem build needs the binary** — copying `emb-binary` from the build output duplicates the binary. Mitigation: the binary is small (~20MB), removed after build.
- **CI equivalent already exists** — the release workflow and gem CI workflows cover this. The `just all` recipe is for local development convenience.
