## 1. Move gem directory

- [x] 1.1 Move `gem/` → `gems/emb/` preserving all files including `vendor/bundle/`
- [x] 1.2 Remove `.bundle/config` (auto-generated at new location)
- [x] 1.3 Run `bundle install` in the new location to regenerate `.bundle/config` and `Gemfile.lock`

## 2. Add Rakefile

- [x] 2.1 Create `gems/emb/Rakefile` with `require "rspec/core/rake_task"`, `RSpec::Core::RakeTask.new(:spec)`, `task default: :spec`

## 3. Add README

- [x] 3.1 Create `gems/emb/README.md` with prerequisites, setup, E2E test instructions (`rake`), and usage examples

## 4. Add standalone CI

- [x] 4.1 Create `gems/emb/.github/workflows/test.yml` that checks out repo, sources versions, installs ORT + libtokenizers, builds `emb`, starts server, and runs `cd gems/emb && bundle exec rake`
