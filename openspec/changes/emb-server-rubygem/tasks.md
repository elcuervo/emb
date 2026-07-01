## 1. Scaffold gem directory

- [x] 1.1 Create `gems/emb-server/` with `emb-server.gemspec`, `Gemfile`, `VERSION`
- [x] 1.2 Create `gems/emb-server/lib/emb-server/version.rb`
- [x] 1.3 Write gemspec with platform auto-detection from `RUBY_PLATFORM` (overrideable via `EMB_PLATFORM`), `onnxruntime` runtime dependency, `bin/emb` executable

## 2. bin wrapper

- [x] 2.1 Create `gems/emb-server/bin/emb` — finds onnxruntime gem lib, exec's `emb-binary -ort-lib <path>` with user's args

## 3. Integration with release pipeline

- [x] 3.1 Add `release-gem` job to `.github/workflows/release.yml`, builds platform gems per arch, pushes to rubygems.org
