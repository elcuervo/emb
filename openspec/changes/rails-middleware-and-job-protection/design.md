## Context

See proposal.md "Why" for motivation. Current client state: `Emb::Middleware` is a
Rack middleware that clears the per-thread batch scope (`BatchLoader::Executor.clear_current`)
in an `ensure`; batching is on by default so every embedding call in a lazy scope
creates thread-local state; the server signals saturation with
`ERR busy: max concurrent requests exceeded (N)` and the client currently lets that
error propagate into application code; there is no job-processor boundary and no
client-side overload control. Requirements for this change live in
`specs/ruby-batch-loading` (job-scoped clearing), `specs/ruby-client-framework-integration`
(Railtie wiring), and `specs/ruby-client-overload-protection` (cap + retry).

## Goals / Non-Goals

**Goals:**
- One primitive for scope clearing shared by request and job boundaries
- A Railtie that wires everything in a Rails app with two opt-outs, plus
  documented manual paths for non-Rails apps
- Client-side throttle + retry implementable with stdlib only (no new dependencies)
- Specs runnable without a full Rails/Sidekiq install (CI stays lean)

**Non-Goals:**
- Server-side changes (none needed)
- Throttling or serializing `EMB.MULTI` sends — each request/job scope is
  bounded by its own lifecycle; command ordering stays untouched

## Decisions

### D1. One scope-clearing primitive: `Emb::BatchScope.wrap`

Both middlewares plus the ActiveJob hook delegate to a single function:

```ruby
module Emb
  module BatchScope
    def self.wrap
      yield
    ensure
      BatchLoader::Executor.clear_current
    end
  end
end
```

- `Emb::Middleware#call(env)` → `BatchScope.wrap { @app.call(env) }`
- `Emb::JobMiddleware#call(worker, job, queue)` → `BatchScope.wrap { yield }`
- ActiveJob hook → `ActiveJob::Base.around_perform { |_job, block| BatchScope.wrap { block.call } }`

Alternative rejected: one middleware class handling both Rack (`call(env)`) and
job (`call(worker, job, queue)`) arities via argc sniffing — smelly, breaks each
framework's expectations. Two thin classes + one primitive is clearer.

### D2. Framework detection: verify, don't assume

Verified against current sources while exploring:
- **ActiveJob does NOT wrap `perform` in `ActiveSupport::Executor`** (execution.rb
  calls `perform_now` directly inside callbacks). Executor-hook-based clearing would
  leak on adapters that don't wrap (test adapter, `perform_now`).
- **SolidQueue always executes through ActiveJob** and additionally wraps in
  `app_executor` (configurable); Sidekiq's Rails integration wraps via
  `Rails.application.reloader` → `ActiveSupport::Executor`. Both are covered by one
  `ActiveJob::Base.around_perform` registration.

Hence: register the ActiveJob perform callback (covers SolidQueue, Sidekiq-via-AJ,
Shoryuken-via-AJ, async, test), *plus* explicit server middlewares for plain
(non-ActiveJob) Sidekiq and Shoryuken workers. An `ActiveSupport::Executor`
`to_complete` hook was considered and rejected: it is *more* magical (depends on
reloader→executor internal wiring), covers less of the ActiveJob surface, and
overlaps the Rack middleware for requests.

### D3. The Railtie is the single wiring point (sentry-rails style)

Registration lives directly in the Railtie's initializer and
`after_initialize` block — no intermediate integration layer. This mirrors
sentry-rails, which inserts its middleware straight from its railtie
initializer. Testability is handled by the fake-Rails boot harness in
`railtie_spec.rb`, which boots the real Railtie against fake Rails/Sidekiq/
Shoryuken objects and asserts the wiring end-to-end; an extracted
`Emb::Integrations` module was tried and dropped as a testability-only
abstraction with no behavioral value.

- Middleware insertion checks `app.middleware.include?(Emb::Middleware)`
  before `use` so a manual mount is never duplicated.
- `ActiveJob::Base.around_perform` registration happens via
  `ActiveSupport.on_load(:active_job)` so it applies whether ActiveJob loads
  before or after the railtie initializer runs.
- Sidekiq/Shoryuken server-middleware registration is conditional on
  `defined?(Sidekiq)`/`defined?(Shoryuken)` in `after_initialize`.

### D4. Railtie wiring and load-order safety

- `emb.rb` conditionally requires `emb/railtie` (`if defined?(Rails::Railtie)`) —
  standard Rails boot order loads `rails/all` before `Bundler.require`, so the guard
  holds; README documents `require "emb/railtie"` for non-standard orders.
- `Emb::Railtie < Rails::Railtie` with one initializer:
  - `config.emb.middleware = true` (default): `app.middleware.use Emb::Middleware`
    (idempotency-guarded)
  - `config.emb.job_middleware = true` (default): via `ActiveSupport.on_load(:active_job)`
    for the perform callback, `Sidekiq.configure_server` if `defined?(Sidekiq)`,
    `Shoryuken.configure_server` if `defined?(Shoryuken)`
  - Raw config access uses `ActiveSupport::OrderedOptions` on `config.emb`.
- Middleware position: appended via `config.middleware.use` (innermost). Functional
  position is irrelevant — the clear runs in `ensure` and a fresh scope starts
  lazily — but if an app reorders the stack with `insert_before`/`use`, behavior is
  unchanged either way.

### D5. No send-path changes — batching scopes are the whole story

The `EMB.MULTI` send paths (`BATCH_BLOCK` and `Emb.multi`) are deliberately
left untouched: batch-loader already chunks at `batch_size` pairs per command,
and the per-request/per-job scope boundary is what keeps embedding volume and
memory bounded. Alternatives considered and rejected: a throttling/counting
semaphore and retry-on-busy (extra machinery for a goal that is purely about
lifecycle scoping), and a process-wide send mutex (serializes unrelated
workloads for a load guarantee the scope boundary already provides).

## Risks / Trade-offs

- **Rails boot-order variance** → `defined?(Rails::Railtie)` is the 95% case
  (verified standard boot order); escape hatch documented (`require "emb/railtie"`).
  Non-Rails Sidekiq/Shoryuken users get a one-liner in their processor initializer.
- **Double-wrapping ActiveJob jobs under Sidekiq** (server middleware + perform
  callback both fire) → Clearing is idempotent (`clear_current` is a thread-local
  assignment); no semantic cost, noted in tests.
- **Existing `Emb::Middleware` users with manual insertion** → Idempotent guard in
  `install_rack_middleware!`; no duplication.
- **Ruby 3.3+ / stdlib-only constraint** → The scope primitive and middlewares use
  only `BatchLoader::Executor.clear_current`; no new gem dependencies (a stated
  non-goal of the gem's design).

## Migration Plan

1. Implement in `gems/emb` behind the existing gem structure; the Railtie's
   default behavior is opt-out (`config.emb.middleware`/`config.emb.job_middleware`),
   all send-path behavior is unchanged.
2. Bump gem version (minor: new features; no API signatures change).
3. README: replace manual Rails mount guidance with the auto-install story (manual
   remains valid), fix the stale `batch` default claim.
4. Release; no server changes; rollback is reverting this change.

## Open Questions

None.