## 1. Fix the Railtie

- [x] 1.1 Remove the `app.middleware.include?(Emb::Middleware)` guard from the `emb.middleware` initializer in `gems/emb/lib/emb/railtie.rb`; keep the `config.emb.middleware = false` opt-out. Verify `grep -n "include?(Emb::Middleware)" gems/emb/lib/emb/railtie.rb` returns nothing.
- [x] 1.2 Confirm no other code in `gems/emb/lib/` calls inspection methods on `app.middleware` or `config.middleware`; verify with `grep -rn "app.middleware\|config.middleware" gems/emb/lib/`.

## 2. Documentation

- [x] 2.1 In `gems/emb/README.md`, remove the "the Railtie skips insertion when it is already present" claim and document that manual mounts require `config.emb.middleware = false` (duplicates are safe but unnecessary). Verify the manual-wiring paragraph no longer asserts auto-deduplication.

## 3. Real-Rails integration suite

- [x] 3.1 Add `gem 'rails', ENV.fetch('RAILS_VERSION', '~> 8.0'), require: false` to `gems/emb/Gemfile` (development-only; do not add it to the gemspec). Verify `RAILS_VERSION='~> 8.0' bundle install` succeeds in `nix develop`.
- [x] 3.2 Create a minimal dummy app at `gems/emb/spec/integration/dummy/config/application.rb` (boots with `action_controller/railtie`, `eager_load = false`, quiet logger, no public-file server). Verify `ruby -e "require '<abs path>'` boots without error in a subprocess.
- [x] 3.3 Add `gems/emb/spec/integration/helper.rb` that requires `emb` and RSpec config only — no `Emb.setup`/ping — so the suite runs without the emb server. Verify `bundle exec rspec spec/integration` runs with no server listening.
- [x] 3.4 Add `gems/emb/spec/integration/railtie_boot_spec.rb` booting the dummy app in a subprocess and asserting the built `ActionDispatch::MiddlewareStack` includes `Emb::Middleware` exactly once and boot does not raise. Verify the example passes.
- [x] 3.5 Extend the boot spec: with `config.emb.middleware = false` the built stack excludes the middleware; with opt-out plus a manual mount it includes it exactly once. Verify both examples pass.
- [x] 3.6 Extend the boot spec: a request through the app leaves the batch scope cleared on success and on raise (assert `BatchLoader::Executor.current` is nil afterwards). Verify both examples pass.
- [x] 3.7 Add a negative regression guard: temporarily re-adding an `app.middleware.include?` guard makes the boot spec fail. Verify the failure locally, then restore the fix.

## 4. Fake hardening

- [x] 4.1 Delete `RailtieFakeRails::MiddlewareStack#include?` and remove the middleware-mount examples from `gems/emb/spec/emb/railtie_spec.rb`, leaving it focused on ActiveJob/Sidekiq/Shoryuken wiring. Verify `bundle exec rspec spec/emb/railtie_spec.rb` passes.
- [x] 4.2 Add a contract example asserting `RailtieFakeRails::MiddlewareStack.instance_methods(false) - Rails::Configuration::MiddlewareStackProxy.instance_methods` is empty (skip with an explicit pending message only if Rails is unavailable). Verify it passes and fails when a fake-only method (e.g. `include?`) is reintroduced.

## 5. Rake and CI wiring

- [x] 5.1 Add a `spec:rails` RSpec rake task in `gems/emb/Rakefile` scoped to `spec/integration/**/*_spec.rb`. Verify `bundle exec rake spec:rails` runs only the integration examples.
- [x] 5.2 Confirm the default `rake` task still runs every spec (integration included) by checking `bundle exec rake --tasks` and running `bundle exec rake` with the server available.
- [x] 5.3 Add a Ruby CI job to `.github/workflows/ci.yml`: Ruby 3.4 (dev-shell version) across two `RAILS_VERSION` lines (`~> 7.2.0`, `~> 8.0`), running `bundle install` + `bundle exec rake spec:rails` in `gems/emb`. Verify the workflow YAML parses and the job resolves.

## 6. Validation

- [x] 6.1 Inside `nix develop`, run `just test` and (with the server up) `cd gems/emb && bundle exec rake`; confirm the full suite including the new integration examples passes and that the files changed by this change are rubocop-clean (`bundle exec rubocop lib/emb/railtie.rb spec/emb/railtie_spec.rb spec/integration Rakefile`). Pre-existing offenses from the unarchived `resp3-protocol` change are out of scope.
- [x] 6.2 Run `openspec validate rails-railtie-boot-safety --strict` and confirm it passes.
