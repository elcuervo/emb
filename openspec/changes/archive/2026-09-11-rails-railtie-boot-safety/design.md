## Context

See `proposal.md` for motivation. The relevant current state:

- `Emb::Railtie`'s `emb.middleware` initializer runs during `Rails::Application`
  initialization, before `:build_middleware_stack`. At that moment
  `app.middleware` is `Rails::Engine`'s `delegate :middleware, to: :config`, i.e. a
  `Rails::Configuration::MiddlewareStackProxy`. It records `use`/`insert_before`/…
  as lambdas and exposes no inspection API. It is replaced by the real
  `ActionDispatch::MiddlewareStack` only later, when `Rails::Engine#app` runs
  (`engine.rb` assigns `config.middleware = build_middleware.merge_into(stack)`).
- `ActionDispatch::MiddlewareStack` does support `include?` (it includes
  `Enumerable`, and its `Middleware#==` compares to a class), which is what the
  Railtie author expected — but never at initializer time.
- `Emb::BatchScope.wrap` is `yield ensure BatchLoader::Executor.clear_current`, so
  nested wrapping clears an already-clear thread-local. Duplicate middleware is
  functionally inert, not harmful.
- The only "Rails" coverage is `spec/emb/railtie_spec.rb`, which stubs a
  hand-rolled `RailtieFakeRails`. Its `MiddlewareStack` implements `include?`,
  an API the real proxy lacks. Rails is not a dependency anywhere in `gems/emb`,
  and the gem suite is absent from `.github/workflows/ci.yml`.

## Goals / Non-Goals

**Goals:**
- Boot a real Rails application in tests, so the real proxy and the real boot
  order are exercised.
- Make it impossible for a test double to reintroduce this class of failure by
  inventing an API.
- Keep the fast path (`rake` / `just all`) covering the new suite without requiring
  a running emb server.

**Non-Goals:**
- Restoring middleware deduplication against a manual pre-boot mount. It is not
  reachable through Rails' public pre-build API; see Decisions.
- RESP3/other unrelated gem work.
- Replacing the whole fake harness. Real Sidekiq/Shoryuken are heavy optional
  integrations and stay faked for the job-wiring assertions.

## Decisions

### 1. Drop the `include?` guard; make insertion unconditional

Insert `Emb::Middleware` whenever the opt-out flag is not set. Alternatives and
why they were rejected:

- *Inspect `@operations`* — `protected`, and Rails 8 stores opaque lambdas with no
  recoverable class identity.
- *Replay operations onto a probe `ActionDispatch::MiddlewareStack` via
  `merge_into`* — operations like `insert_before ActionDispatch::Static, X` raise
  on an unseeded stack, and `merge_into` is `# :nodoc:`.
- *Deduplicate in `after_initialize`* — runs after `:build_middleware_stack`, so
  the stack is already built.
- *Runtime no-op wrapper* — hides duplicates rather than preventing them, and adds
  complexity for no user-visible benefit.

Since `BatchScope.wrap` is idempotent, unconditional insertion is correct and the
cheapest honest behavior. The manual-mount path is the existing
`config.emb.middleware = false` flag.

### 2. Real-Rails coverage via a minimal dummy app booted in a subprocess

A minimal in-repo application (`spec/integration/dummy`) is booted with
`Rails.application.initialize!`, then assertions read the built stack via
`Rails.application.middleware` (post-boot this is the real
`ActionDispatch::MiddlewareStack`).

The boot runs in a **subprocess** rather than the main RSpec process because:
- `emb.rb` conditionally requires the Railtie, and the fake-based spec
  `load`s/`remove_const`s `Emb::Railtie` per example; loading real Rails in the
  same process makes that constant dance unreliable.
- The existing suite already uses this pattern for the "no Rails present" check.

Alternatives: loading only `railties` and driving the real proxy without a full
boot (lighter, catches the proxy API, but not the boot order or the built stack);
`rails/all` instead of `action_controller/railtie` (heavier for no extra fidelity).

### 3. Separate serverless helper for the integration specs

`spec/integration` uses its own helper (no `Emb.setup`/ping), so the suite can run
without the emb server. The integration specs still live under `spec/`, so the
default `RSpec::Core::RakeTask` pattern picks them up and `just all` / `rake`
cover them. A dedicated `spec:rails` task runs only the integration directory for
fast CI without Go/ONNX.

### 4. Strict-subset contract for the fake

Delete `RailtieFakeRails::MiddlewareStack#include?` and assert that the fake
defines no method the real class lacks:

```ruby
expect(RailtieFakeRails::MiddlewareStack.instance_methods(false) -
       Rails::Configuration::MiddlewareStackProxy.instance_methods).to be_empty
```

This encodes the root cause as a check: a stand-in may be *less* capable than the
real object, never more.

### 5. CI matrix and dependency placement

Add Rails as a development dependency only (Gemfile, not the gemspec `add_dependency`),
selected via `RAILS_VERSION` (default `~> 8.0`). Add a Ruby CI job on the dev-shell
Ruby (3.4) across two supported Rails lines (`~> 7.2.0` and `~> 8.0`), running
`rake spec:rails`.

## Risks / Trade-offs

- [Rails in the gem Gemfile slows `bundle install` for contributors] → it is
  development-only; the gem's runtime dependencies are unchanged.
- [Subprocess boot cost per example] → keep the dummy app minimal and boot once
  per example group where possible; total budget well under a second.
- [Integration helper drifts from `spec_helper` server setup] → the integration
  helper deliberately does not touch the server; it requires only `emb` and
  RSpec.
- [Version matrix maintenance] → two supported Rails lines, both blocking.
- [Fake contract test itself needs real Rails loaded] → the real-Rails dev
  dependency guarantees it is available; if Rails is absent the example is skipped
  with an explicit pending message rather than silently passing.

## Migration Plan

1. Land the Railtie fix and README correction (behavior is then correct).
2. Replace the fake-based middleware assertions with the real boot suite.
3. Add the CI job. Rollback is a revert of the change; the opt-out flag keeps
   manual control available throughout.

## Open Questions

None.
