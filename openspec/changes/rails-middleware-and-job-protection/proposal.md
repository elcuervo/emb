## Why

The Ruby client's lazy batching is per-thread: in any long-lived thread — a Rails
request thread, a Sidekiq/SolidQueue worker — cached embeddings and unflushed
loaders accumulate for the life of the thread. Today only Rack apps get scope
hygiene (`Emb::Middleware`), and only by manual wiring; job processors have no
boundary at all, so worker threads leak per-job cache memory and cross-contaminate
batch scopes between jobs: a job's unused loaders can silently ride along on the
next job's flush, and cached vectors linger across jobs. Rails users also still
have to mount the middleware by hand despite batching being on by default. The
goal is batching by default, cleanly scoped to request and worker lifecycles.

## What Changes

- **New Railtie** (`Emb::Railtie`) that auto-installs `Emb::Middleware` in the Rails
  middleware stack and auto-registers job-scope protection for ActiveJob, Sidekiq,
  and Shoryuken server processes — zero-config opt-in, with `config.emb.middleware`
  and `config.emb.job_middleware` opt-outs.
- **New `Emb::JobMiddleware`**: a framework-agnostic middleware that clears the
  per-thread batch scope after each job execution, mirroring `Emb::Middleware`'s
  guarantee for requests (cleared even on exception). Registered automatically for
  ActiveJob (via `around_perform` on `ActiveJob::Base` — covering SolidQueue and
  every ActiveJob adapter), Sidekiq server middleware, and Shoryuken server
  middleware.
- **Shared scope-clear helper** (`Emb::BatchScope`) used by both middlewares so
  request and job boundaries guarantee identical semantics.
- **README fixes**: correct the stale "default is `false`" claim about the `batch`
  option, and document auto-installation and opt-outs.

## Capabilities

### New Capabilities
- `ruby-client-framework-integration`: Rails/background-framework integration —
  Railtie auto-install of the Rack middleware, auto-registration of job-scope
  protection for ActiveJob, Sidekiq, and Shoryuken, and opt-out configuration.

### Modified Capabilities
- `ruby-batch-loading`: scope-clearing semantics extend from Rack requests to job
  executions — `Emb::JobMiddleware` gives background work the same per-scope
  clearing guarantee that `Emb::Middleware` gives requests.

## Impact

- **Gem**: `gems/emb/lib/emb/{middleware.rb,railtie.rb,job_middleware.rb,batch_scope.rb,configuration.rb}`,
  `gems/emb/lib/emb.rb` (guarded railtie require), README.
- **Specs**: `gems/emb/spec/emb_batch_spec.rb` (existing Middleware suite extended),
  new spec files for the railtie (fake-Rails boot harness) and job middleware.
- **Dependencies**: none new — Rails/Sidekiq/ActiveJob remain optional runtime
  integrations detected at load time.
- **Behavioral note**: batching stays on by default; per-request and per-job batch
  scopes are cleared at each boundary, so cached embeddings and unused loaders
  never leak between requests or jobs.