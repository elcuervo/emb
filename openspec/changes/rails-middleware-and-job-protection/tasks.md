## 1. Shared scope primitive and job middleware

- [x] 1.1 Add `Emb::BatchScope.wrap` (yield, clear per-thread scope in ensure) and refactor `Emb::Middleware#call` to delegate to it; verify the existing `Emb::Middleware` suite in `spec/emb_batch_spec.rb` still passes (`bundle exec rspec spec/emb_batch_spec.rb`)
- [x] 1.2 Add `Emb::JobMiddleware` with `call(worker, job, queue)` delegating to `Emb::BatchScope.wrap`; verify new specs covering 'Job-scoped cache clearing' scenarios: scope cleared after a job, cleared when the job raises, unused loaders dropped at job end, return value/exception passthrough through yield

## 2. Framework integration (single Railltie wiring point)

- [x] 2.1 Add `Emb::Railtie` (file guarded by `defined?(Rails::Railtie)`, `config.emb` OrderedOptions defaults `middleware: true`/`job_middleware: true`) that inlines all wiring sentry-rails style: idempotent `app.middleware.use Emb::Middleware` initializer, ActiveJob `around_perform` via `ActiveSupport.on_load(:active_job)`, conditional `Sidekiq`/`Shoryuken` server-middleware registration in `after_initialize`; verify `emb.rb` conditional require loads cleanly with no Rails present
- [x] 2.2 Verify the Railtie with the fake-Rails boot harness in `railtie_spec.rb` (load order, middleware mounting, ActiveJob/Sidekiq/Shoryuken registration, both opt-outs) — this harness is the single test surface for registration (no separate integrations module/spec)

## 3. Documentation

- [x] 3.1 Update README: Rails auto-install story replacing the manual-mount-default, `config.emb.*` opt-outs, non-Rails manual wiring one-liners; verify claims match implemented behavior and no stale instructions remain

## 4. Validation

- [x] 4.1 Run `openspec validate rails-middleware-and-job-protection` (strict) and fix any spec/requirement issues
- [x] 4.2 Run the full gem suite (`cd gems/emb && bundle exec rake` against the server on :16379) plus `bundle exec rubocop`; verify green and all new specs included
- [x] 4.3 Confirm no new gem dependencies (`bundle check`) and gem still builds (`gem build emb.gemspec`)