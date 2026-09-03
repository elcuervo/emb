## ADDED Requirements

### Requirement: Job-scoped cache clearing

The gem SHALL provide `Emb::JobMiddleware`, a background-job middleware that clears
the per-thread batch scope after each job execution via `BatchLoader::Executor.clear_current`,
mirroring the guarantee `Emb::Middleware` provides for Rack requests. Clearing SHALL
happen even when the job raises, and each job SHALL start with a fresh batch scope.
The middleware SHALL be execution-framework agnostic: it SHALL wrap a job callback
and yield to the job body, so the same class can be registered in any job processor
(ActiveJob, Sidekiq, Shoryuken, and adapters built on them such as SolidQueue).

#### Scenario: Scope is cleared after each job

- **WHEN** `Emb::JobMiddleware` wraps a job body that creates and uses lazy embeddings
- **THEN** the batch scope SHALL be empty after the job completes
- **AND** a second job on the same thread SHALL start with a fresh scope (the same pair re-sends `EMB.MULTI`)

#### Scenario: Scope is cleared when the job raises

- **WHEN** `Emb::JobMiddleware` wraps a job body that raises after creating lazy embeddings
- **THEN** the per-thread batch scope SHALL still be cleared

#### Scenario: Unused loaders are dropped at job end

- **WHEN** a job creates lazy loaders that are never used
- **THEN** no `EMB.MULTI` SHALL be sent for them
- **AND** they SHALL be dropped when the job ends, not carried into the next job

#### Scenario: Middleware yields to the job body

- **WHEN** `Emb::JobMiddleware` wraps job execution with `call(worker, job, queue)`
- **THEN** it SHALL invoke the rest of the middleware chain (yield) and the job body
- **AND** it SHALL pass through the job body's return value or exception unchanged