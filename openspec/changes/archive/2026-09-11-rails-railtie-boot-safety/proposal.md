## Why

The `emb` Railtie's middleware initializer calls
`app.middleware.include?(Emb::Middleware)`, but during boot `app.middleware` is a
`Rails::Configuration::MiddlewareStackProxy` — a command recorder with no
`include?` — so the guard raises `NoMethodError` in every real Rails application.
It was never caught because the only "Rails" test uses a hand-rolled fake whose
middleware stack implements `include?`, an API the real proxy has never had
(verified against Rails 7.0–8.0). The gem's Rails integration has therefore never
worked in production, and no test can tell.

## What Changes

- Remove the impossible `include?` guard from `Emb::Railtie`'s `emb.middleware`
  initializer; insert `Emb::Middleware` unconditionally. Opt-out stays
  `config.emb.middleware = false`.
- Replace the "manual insertion is not duplicated" guarantee — unimplementable
  through Rails' pre-build middleware API — with a truthful one: insertions are
  additive, and duplicate insertion is **safe** because `Emb::BatchScope.wrap`'s
  clear is idempotent. Document `config.emb.middleware = false` as the manual-mount
  path.
- Correct the README's false claim that the Railtie "skips insertion when it is
  already present" (it raises before reaching that point).
- Add a real-Rails boot test suite: Rails as a development dependency, a minimal
  dummy app, and integration specs that boot it in a subprocess and assert against
  the **built** `ActionDispatch::MiddlewareStack` (auto-insert exactly once, opt-out,
  manual mount, request/job scope clearing).
- Harden the fake: delete its invented `include?` and add a strictness contract so a
  test fake can never expose a method the real Rails class lacks.
- Add a Ruby CI job that runs the Rails boot suite across supported Ruby and Rails
  versions. The suite stays inside the default `rake` pattern so `just all` covers
  it too.

## Capabilities

### New Capabilities
<!-- None: this change fixes an existing capability and its validation. -->

### Modified Capabilities
- `ruby-client-framework-integration`: the Railtie middleware-insertion requirement loses the unimplementable "SHALL NOT be duplicated" guarantee; insertion becomes unconditional and explicitly safe when duplicated.
- `gems-emb-ci`: adds real-Rails boot validation to the `emb` gem's test/CI requirements (dev Rails dependency, dummy-app integration suite, supported Rails/Ruby matrix).

## Impact

- **Gem**: `gems/emb/lib/emb/railtie.rb`; `gems/emb/README.md` (Rails integration section).
- **Tests**: `gems/emb/spec/emb/railtie_spec.rb` (rework: job wiring only, fake corrected), new `gems/emb/spec/integration/` (helper, minimal dummy app, Railtie boot spec), `gems/emb/Gemfile` (dev-only `rails`), `gems/emb/Rakefile` (`spec:rails` task).
- **CI**: new Ruby job in `.github/workflows/ci.yml` (the gem suite is not run in CI today; it only runs via `just all` and the release workflow).
- **Dependencies**: Rails becomes a development dependency only — no runtime dependency change, and Rails/ActiveJob/Sidekiq/Shoryuken stay optional at runtime.
- **Behavior**: non-Rails behavior and the opt-out flag are unchanged; a real Rails app stops raising at boot.
