# ruby-batch-loading

## Purpose

Specifies lazy client-side batching for the `emb` gem: embedding calls made within the same
execution scope (thread) coalesce into a single `EMB.MULTI` command using the `batch-loader`
gem, with a `batch` configuration option that makes the default proxy API lazy.

## Requirements

### Requirement: Emb.batch exposes lazy batched embeddings

The gem SHALL expose a module-level `Emb.batch` and an instance-level `client.batch` that
return a per-model lazy proxy sharing the syntax of the standard proxy API:
`Emb.batch[:model]["text"]` returns a lazy embedding that materializes on first use.
The explicit batch API SHALL be available regardless of the `batch` configuration option.
The value SHALL match the eager API's shape: a single text yields an Array of Float,
multiple texts yield an Array of Array of Float.

#### Scenario: Single text is lazy and materializes to a vector

- **WHEN** `loader = Emb.batch[:minilm]["hello world"]` is called
- **THEN** no command SHALL be sent to the server at call time
- **AND** `loader.sum` SHALL send `EMB.MULTI minilm "hello world"` to the server
- **AND** `loader.sum` SHALL return the sum of the embedding for `minilm`/`"hello world"`

#### Scenario: Multi-text matches eager shape

- **WHEN** `vecs = Emb.batch[:minilm]["hello", "world"]` is used
- **THEN** `vecs` SHALL be an Array of Array of Float

#### Scenario: Instance client batch API

- **WHEN** `client = Emb.new; client.batch[:minilm]["hello"]` is used
- **THEN** it SHALL send `EMB.MULTI` to the client's configured server

### Requirement: Per-scope coalescing into EMB.MULTI

All lazy embeddings created in the same execution scope (thread) and batch scope SHALL be
delivered to the server as `EMB.MULTI` command(s) when the first of them is used,
coalesced per client into chunks of at most the configured `batch_size` pairs each.
Embeddings for different models SHALL coalesce into the same command(s), preserving
per-pair order in the response. Creating loaders SHALL NOT cause I/O; using a value
triggers the flush. Chunking SHALL be unconditional (not triggered by server errors) so a
single command never exceeds the server's `max_pairs` cap and stays within typical client
read timeouts.

#### Scenario: Same-model loaders coalesce into one MULTI

- **WHEN** `l1 = Emb.batch[:minilm]["a"]`, `l2 = Emb.batch[:minilm]["b"]`, and `l3 = Emb.batch[:minilm]["c"]` are created in the same scope
- **AND** `l1.sum` is called
- **THEN** a single command `EMB.MULTI minilm "a" minilm "b" minilm "c"` SHALL be sent to the server
- **AND** `l2.sum` and `l3.sum` SHALL return the correct embeddings without additional commands

#### Scenario: Mixed-model loaders coalesce into one MULTI

- **WHEN** `Emb.batch[:minilm]["a"]` and `Emb.batch[:bge]["b"]` are created and used in the same scope
- **THEN** a single command `EMB.MULTI minilm "a" bge "b"` SHALL be sent
- **AND** each value SHALL be the embedding from its own model

#### Scenario: Loaders created after a flush form a new batch

- **WHEN** a batch has already been flushed in the scope
- **AND** a new loader is then created and used
- **THEN** a new `EMB.MULTI` command SHALL be sent containing only the new loader's pairs

#### Scenario: Large scope resolves in chunked commands

- **GIVEN** a client configured with `batch_size` 100
- **WHEN** a scope defers 250 pairs
- **THEN** the scope resolves via three `EMB.MULTI` commands with 100, 100, and 50 pairs respectively
- **AND** results are returned in the deferral order with single-text values as vectors and multi-text values as collections, exactly as with one command

#### Scenario: Chunked failures keep MGET semantics

- **GIVEN** a client configured with `batch_size` 100
- **WHEN** a scope defers pairs including an unknown model, spanning two chunks
- **THEN** each chunk resolves with per-pair `nil` for failed pairs and the loader returns values in deferral order

#### Scenario: batch_size is configurable

- **WHEN** `Emb.configure { |c| c.batch_size = 64 }` is set before clients are created
- **THEN** all clients created afterwards use 64-pair chunks
- **AND** an explicit per-client `batch_size:` option overrides the global setting

### Requirement: Cached values within a scope

The gem SHALL cache loaded embeddings within the execution scope (`cache: true`), so using
the same lazy embedding more than once SHALL NOT trigger additional server commands.

#### Scenario: Repeat use does not re-send

- **WHEN** `loader = Emb.batch[:minilm]["hello"]` is created and `loader.sum` is called twice
- **THEN** exactly one `EMB.MULTI` command SHALL be sent to the server

#### Scenario: Identical pairs deduplicate

- **WHEN** `a = Emb.batch[:minilm]["dup"]` and `b = Emb.batch[:minilm]["dup"]` are created and both used in the same scope
- **THEN** a single `EMB.MULTI` SHALL be sent with one pair for `minilm`/`"dup"`
- **AND** `a` and `b` SHALL materialize to the same value

### Requirement: Batch mode configuration

The gem SHALL accept a `batch` option in its client configuration, defaulting to `true`.
When `true` (the default), the standard proxy API (`Emb[:model]["text"]` / `client[:model]["text"]`)
SHALL return lazy batched embeddings instead of sending commands immediately.
When `false`, the standard proxy API SHALL send `EMB` immediately (eager), exactly as
before batching was introduced.

#### Scenario: Default is lazy

- **WHEN** `client = Emb.new` is created without a `batch` option
- **THEN** `client[:minilm]["hello"]` SHALL NOT send a command at call time
- **AND** using the returned value SHALL send `EMB.MULTI` and return an Array of Float

#### Scenario: Batch mode makes the proxy lazy (explicit)

- **WHEN** `client = Emb.new(batch: true)` is created
- **THEN** `client[:minilm]["hello"]` SHALL NOT send a command at call time
- **AND** using the returned value SHALL send `EMB.MULTI` and return an Array of Float

#### Scenario: Opt-out restores eager sends

- **WHEN** `client = Emb.new(batch: false)` is created
- **THEN** `client[:minilm]["hello"]` SHALL send `EMB` immediately and return an Array of Float

### Requirement: Failure handling follows MGET semantics

A pair whose embedding fails (unknown model or inference error) SHALL materialize as `nil`,
mirroring `EMB.MULTI`'s per-pair null behavior. Successful pairs in the same batch SHALL
materialize normally.

#### Scenario: Failed pair yields nil, siblings succeed

- **WHEN** `a = Emb.batch[:minilm]["ok"]` and `b = Emb.batch[:ghost]["nope"]` are created in the same scope
- **AND** the server returns a null for the `ghost` pair
- **AND** `a` and `b` are used
- **THEN** `a` SHALL be an Array of Float
- **AND** `b.nil?` SHALL be `true`

#### Scenario: Checkable without surprising callers

- **WHEN** a failed pair's loader is used
- **THEN** the usage SHALL behave like operating on `nil`, consistent with the existing `Emb.multi` partial-failure behavior

### Requirement: Request-scoped cache clearing

The gem SHALL provide `Emb::Middleware`, a Rack middleware that clears the per-thread batch
scope after each request via `BatchLoader::Executor.clear_current`. Clearing SHALL happen
even when the wrapped application raises, and SHALL NOT leak cached values or pending
loaders into the next request.

#### Scenario: Middleware clears the scope after each request

- **WHEN** `Emb::Middleware` wraps an application in the middleware stack
- **AND** the first request creates and uses `Emb.batch[:minilm]["hello"]`
- **AND** the second request creates and uses `Emb.batch[:minilm]["hello"]` again
- **THEN** a new `EMB.MULTI` command SHALL be sent for the second request
- **AND** the first request's cached value SHALL NOT be reused

#### Scenario: Scope is cleared when the application raises

- **WHEN** the wrapped application raises an exception
- **THEN** the per-thread batch scope SHALL still be cleared
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

- **WHEN** `Emb::JobMiddleware` wraps job execution with the framework's middleware signature (e.g. `call(worker, job, queue)` for Sidekiq, `call(worker, queue, sqs_msg, body)` for Shoryuken)
- **THEN** it SHALL invoke the rest of the middleware chain (yield) and the job body regardless of the number of positional arguments
- **AND** it SHALL pass through the job body's return value or exception unchanged

### Requirement: Batch failures retry then fail closed

When a batch command fails with a transient error (timeout, connection error, protocol error), the gem SHALL re-send the command up to `reconnect_attempts` additional times (the first attempt plus `reconnect_attempts` retries in total) before failing closed. Operation errors — replies the server parsed and rejected (`RedisClient::CommandError`, e.g. unknown model) — SHALL NOT be retried. When a batch fails after all retries (or immediately for an operation error or the default `reconnect_attempts: 0` configuration), the gem SHALL raise `Emb::ServerError` to the code that forced the batch, SHALL remove every deferred item of that batch from the scope's pending set, and SHALL NOT re-send the failed batch on any later resolution attempt. `Emb::ServerError` SHALL carry the underlying redis error as `cause`, plus the model(s), text count, and attempt count. Re-resolving an item from a failed batch SHALL return an empty array (`[]`) and SHALL NOT perform I/O. New batches created in the same scope afterwards SHALL contain only items deferred after the failure.

#### Scenario: Error surfaces once and pending is cleared

- **WHEN** a scope defers 6 items and the `EMB.MULTI` for them fails under the default configuration (`reconnect_attempts: 0`)
- **THEN** the forced value SHALL raise `Emb::ServerError` after a single attempt
- **AND** the scope's pending set SHALL be empty afterwards

#### Scenario: Transient failure recovers within retries

- **GIVEN** a client configured with `reconnect_attempts: 2`
- **WHEN** a scope defers 6 items and the first two `EMB.MULTI` sends for them time out
- **AND** the third send succeeds
- **THEN** the forced loader SHALL materialize the real embeddings for all 6 items
- **AND** the server SHALL have received exactly 3 `EMB.MULTI` commands for the batch

#### Scenario: Exhausted retries raise a typed error and clear pending

- **GIVEN** a client configured with `reconnect_attempts: 2`
- **WHEN** a scope defers 6 items and every `EMB.MULTI` send for them fails transiently
- **THEN** the forced value SHALL raise `Emb::ServerError`
- **AND** 3 attempts SHALL have been sent to the server (`reconnect_attempts + 1`)
- **AND** the scope's pending set SHALL be empty afterwards

#### Scenario: Operation errors are not retried

- **GIVEN** a client configured with `reconnect_attempts: 2`
- **WHEN** a scope defers items and the server returns an error reply (not a timeout/connection error) on the first send
- **THEN** the forced value SHALL raise `Emb::ServerError`
- **AND** exactly one command SHALL have been sent to the server

#### Scenario: ServerError carries failure context

- **WHEN** a batch fails (after exhausting retries, or on the first attempt)
- **THEN** the raised `Emb::ServerError`'s `cause` SHALL be the underlying redis error
- **AND** its message SHALL include the model(s), the text count, and the number of attempts

#### Scenario: Retry resolves to [] without I/O

- **WHEN** an item of a failed batch is resolved again after the failure
- **THEN** it SHALL return an empty array (`[]`)
- **AND** no command SHALL be sent to the server

#### Scenario: Subsequent batches exclude failed items

- **WHEN** a batch has failed in a scope and the scope then defers 2 new items which are forced
- **THEN** the server SHALL receive a single command containing exactly the 2 new pairs
- **AND** none of the previously failed items SHALL be re-sent

#### Scenario: Pending set stays bounded across failures

- **WHEN** several batches fail sequentially in the same scope (with or without retries in between)
- **THEN** the pending set SHALL NOT grow beyond the items currently deferred in the scope
