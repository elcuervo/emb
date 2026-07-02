## Context

The `gems/emb/` (client) and `gems/emb-server/` (server distribution) gems have no code style enforcement. They each have a Rakefile (`gems/emb/` does, `gems/emb-server/` doesn't), a Gemfile, and a handful of `.rb` source files. The `just dev` recipe builds and runs the server but has issues with `ort_lib` resolution in some environments.

## Goals / Non-Goals

**Goals:**
- Consistent Ruby style across both gems via shared RuboCop rules
- All existing RuboCop warnings fixed (auto-correct where possible, manual fix otherwise)
- Interactive `irb` console for validating gem behavior without running tests
- `just dev` works reliably (builds server, starts with correct config)

**Non-Goals:**
- Not changing the Go codebase
- Not adding RuboCop to CI (can be added later via the existing CI workflows)
- Not rewriting or refactoring beyond what RuboCop requires

## Decisions

### RuboCop config per gem, not shared root config

Both gems are independent — they have different Gemfiles, different dependencies, different code. Each gets its own `.rubocop.yml` with the same baseline rules. This keeps them self-contained.

### Cops to enable

| Gem | Cops |
|-----|------|
| `emb` | `rubocop` base, `rubocop-rspec`, `rubocop-rake` |
| `emb-server` | `rubocop` base, `rubocop-rake` (no specs yet) |

### Ruby version target

Both gems require `>= 3.3` per their gemspecs. RuboCop `AllCops` will target `3.3`.

### Console via Rake

A `console` Rake task in each gem that runs `irb` with the gem's lib loaded:
```ruby
task :console do
  require "irb"
  require_relative "lib/emb"
  IRB.start
end
```

This is a standard Ruby pattern — simple, zero dependencies, works in any environment.

### `just dev` fix

The current `just dev` recipe references `{{ort_lib}}` which resolves `DYLD_LIBRARY_PATH`. If empty, the `-L` flag is empty and the build/link fails. Fix: provide a fallback or skip the flag when empty. The `config.yaml` at the repo root should be the dev config.

## Risks / Trade-offs

- RuboCop auto-correct may change formatting in non-trivial ways — reviewed manually before committing
- `just dev` requires onnxruntime + libtokenizers — if either is missing the build will fail with a clear linker error (acceptable)
