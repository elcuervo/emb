## Purpose

Integrates the emb Ruby client with Rails and background-job processors automatically: a Railtie mounts `Emb::Middleware` in the Rails middleware stack and registers `Emb::JobMiddleware` for ActiveJob, Sidekiq, and Shoryuken processes with explicit opt-outs, plus documented manual wiring for non-Rails Rack and job setups.

## ADDED Requirements

### Requirement: Rails Railtie auto-installs the Rack middleware

The gem SHALL ship a Rails Railtie that, when the gem is loaded in a Rails application, inserts `Emb::Middleware` into the application's middleware stack, so request-scoped batch clearing is active without manual configuration. Insertion SHALL be idempotent (a manually-inserted `Emb::Middleware` SHALL NOT be duplicated). An operator SHALL be able to opt out via `config.emb.middleware = false`.

#### Scenario: Middleware auto-inserted in a Rails app

- **WHEN** a Rails application loads the gem without manual middleware configuration
- **THEN** the application's middleware stack SHALL include `Emb::Middleware`

#### Scenario: Manual insertion is not duplicated

- **WHEN** an application already inserted `Emb::Middleware` into its middleware stack
- **THEN** the Railtie SHALL NOT insert a second copy

#### Scenario: Opt-out disables auto-insertion

- **WHEN** an application sets `config.emb.middleware = false`
- **THEN** the middleware stack SHALL NOT include `Emb::Middleware` from the Railtie

### Requirement: Railtie auto-registers job-scope protection

When the gem is loaded in a Rails application, the Railtie SHALL register `Emb::JobMiddleware` for every job-processing framework present: ActiveJob (via a perform callback on `ActiveJob::Base`, covering all adapters including SolidQueue), Sidekiq (server middleware, covering plain Sidekiq workers), and Shoryuken (server middleware). Registration SHALL be conditional on the framework being loaded and SHALL be a no-op for frameworks that are not present. An operator SHALL be able to opt out entirely via `config.emb.job_middleware = false`.

#### Scenario: ActiveJob jobs run under the middleware

- **WHEN** a Rails application defines and performs an ActiveJob job with lazy embeddings
- **THEN** the per-thread batch scope SHALL be cleared after the job completes, even on exception

#### Scenario: Sidekiq server middleware is registered

- **WHEN** a Rails application boots with Sidekiq loaded
- **THEN** `Emb::JobMiddleware` SHALL be added to the Sidekiq server middleware chain
- **AND** plain Sidekiq workers SHALL have their batch scope cleared after each job

#### Scenario: Shoryuken server middleware is registered

- **WHEN** a Rails application boots with Shoryuken loaded
- **THEN** `Emb::JobMiddleware` SHALL be added to the Shoryuken server middleware chain

#### Scenario: SolidQueue jobs are protected via ActiveJob

- **WHEN** a Rails application runs jobs through SolidQueue (an ActiveJob backend)
- **THEN** those jobs SHALL execute under the ActiveJob perform callback registered by the Railtie

#### Scenario: Missing frameworks are skipped

- **WHEN** a Rails application has no Sidekiq and no Shoryuken loaded
- **THEN** the Railtie SHALL register only the ActiveJob protection and SHALL not raise

#### Scenario: Job protection opt-out

- **WHEN** an application sets `config.emb.job_middleware = false`
- **THEN** the Railtie SHALL NOT register any job-scope protection

### Requirement: Manual wiring for non-Rails applications

The gem SHALL keep the manual integration paths working and documented: any Rack app mounts `Emb::Middleware` with `use`, and non-Rails Sidekiq or Shoryuken processes register `Emb::JobMiddleware` with the framework's server-middleware configuration. The load of the Railtie and framework integrations SHALL be safe when Rails, Sidekiq, ActiveJob, or Shoryuken are absent.

#### Scenario: Non-Rails Rack app uses the middleware manually

- **WHEN** a non-Rails Rack application mounts `Emb::Middleware`
- **THEN** request-scoped batch clearing SHALL behave as specified

#### Scenario: Non-Rails Sidekiq registers job middleware manually

- **WHEN** a non-Rails Sidekiq process adds `Emb::JobMiddleware` to its server middleware chain
- **THEN** each job SHALL have its batch scope cleared on completion

#### Scenario: Gem loads without any framework

- **WHEN** the gem is required in a plain Ruby process with no Rails, Sidekiq, ActiveJob, or Shoryuken
- **THEN** loading the gem SHALL succeed and SHALL NOT attempt framework integration