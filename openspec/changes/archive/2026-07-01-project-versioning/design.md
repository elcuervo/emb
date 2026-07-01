## Context

Three independent version sources exist. The Go binary version comes from `git describe` (set via ldflags). Each gem has its own `VERSION` file. The release workflow needs to keep all three in sync, especially when building platform gems from the same tag.

## Goals / Non-Goals

**Goals:**
- Single root `VERSION` file read by all consumers
- Gems published with the same version as the Go binary
- Justfile uses root VERSION for ldflags
- CI overwrites root VERSION from tag before builds

**Non-Goals:**
- Semantic version enforcement or validation
- Changelog generation
- Automated version bumps on non-release commits

## Decisions

### Decision: Root VERSION file

```
VERSION  (repo root)
  0.1.0
```

All consumers reference this file. No other version files exist as authoritative sources.

### Decision: Gem VERSION reads root via relative path

Each gem's `VERSION` file:

```ruby
# gems/emb/VERSION
File.read(File.expand_path("../../VERSION", __dir__)).strip
```

Ruby's `File.read` raises `Errno::ENOENT` if the root file is missing — hard failure (intentional).

Previously these files were plain text strings. Changing them to a Ruby expression means the gemspec needs to `require` or `eval` the file. Update: change the gemspec to read the root VERSION directly, and make gem VERSION files simple text again but generated from root.

Actually, simplest approach:

```
VERSION → "0.1.0" (plain text, repo root)

gems/emb/VERSION       → copies root VERSION at build time
gems/emb-server/VERSION → copies root VERSION at build time
```

But for local development, the gem VERSION files should exist. Solutions:

A. **Symlink**: `ln -s ../../VERSION gems/emb/VERSION` — simple, but gemspec can't follow symlinks in all cases.

B. **Copy on build**: gem VERSION files are plain text (committed). CI overwrites them from root VERSION before gem build. Local dev updates all manually.

C. **Gemspec reads root directly**: `File.read("../../VERSION")` relative to the gemspec directory. Works for `gem build` from `gems/emb/`.

Going with **C** for simplicity — the gemspec reads the root VERSION directly. Each gem's `VERSION` file is removed (or kept as a convenience symlink).

### Decision: Justfile reads VERSION file

Replace `git describe --tags --dirty --always` with:

```
image_tag := `cat "$(git rev-parse --show-toplevel)/VERSION" 2>/dev/null || echo "dev"`
```

This reads the root VERSION file directly. `dev` fallback when not in a git checkout.

### Decision: CI version injection

On tag push, the release workflow:

1. Write tag version to root VERSION:
   ```
   echo "${TAG_NAME#v}" > VERSION
   ```
2. Build everything (Go binary, gems) — all pick up the new version
3. Build and push gems

This happens at the start of the workflow, before any build steps.

## Risks / Trade-offs

- **Gemspec reads outside its directory** — `File.read("../../VERSION")` from `gems/emb/` crosses directory boundaries. Works for `gem build` but may confuse some packaging tools.
- **Justfile dependency on git** — `git rev-parse --show-toplevel` requires git. Falls back to `dev`. Acceptable.
- **CI mutation of VERSION file** — the workflow writes to VERSION during a release run. This is ephemeral (within the CI container) and doesn't affect the repo.
