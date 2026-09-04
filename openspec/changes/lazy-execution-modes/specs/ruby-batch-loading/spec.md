## REMOVED Requirements

### Requirement: Emb.batch exposes lazy batched embeddings

**Reason**: The explicit `Emb.batch` / `client.batch` lazy proxy is superseded by the `lazy` mode configuration, which makes the standard proxy API (`Emb[:model][text]`) lazy under `lazy: :multi` or `lazy: :batch`. Two entry points for deferred behavior (a config flag AND an explicit API) created ambiguity about which one governs.

**Migration**: Configure `lazy: :multi` or `lazy: :batch` for deferred proxy behavior, or use `Emb.multi { }` for explicit eager composition. Any code calling `Emb.batch[...]` / `client.batch[...]` must switch to the mode-driven proxy.

### Requirement: Batch mode configuration

**Reason**: The binary `batch` option is replaced by the mutually exclusive `lazy` mode enum (`false` / `:multi` / `:batch`), and the default flips from lazy (`batch: true`) to eager (`lazy: false`) so a default client makes exactly one round trip per embed call.

**Migration**: Replace `batch: true` with `lazy: :multi` (identical coalescing behavior) or `lazy: :batch` (deferred, executed concurrently). Replace `batch: false` with the default (`lazy: false`) or explicit `lazy: false`.

## MODIFIED Requirements

### Requirement: Per-scope coalescing into EMB.MULTI

All lazy embeddings created in the same execution scope (thread) and batch scope under a deferred mode (`lazy: :multi` or `lazy: :batch`) SHALL be delivered to the server when the first of them is used, coalesced per client into chunks of at most the configured `batch_size` texts each. A chunk whose items all use one model SHALL be delivered as a single `EMB <model> <text>...` command (the server packs its texts into one inference). A chunk spanning models SHALL be delivered as one `EMB.MULTI <model> <text>...` command, preserving per-pair nil behavior. Creating loaders SHALL NOT cause I/O; using a value triggers the flush. Chunking SHALL be unconditional (not triggered by server errors) so a single command never exceeds the server's `max_texts`/`max_pairs` cap and stays within typical client read timeouts. In `:batch` mode, the chunk shares are the executable units dispatched concurrently (per the `client-multi-instance-distribution` capability).

#### Scenario: Same-model loaders coalesce into one MULTI

- **WHEN** `a = Emb[:minilm]["x"]`, `b = Emb[:minilm]["y"]`, and `c = Emb[:minilm]["z"]` are created in the same scope under `lazy: :multi` and `a` is used
- **THEN** a single command `EMB minilm "x" "y" "z"` SHALL be sent to the server
- **AND** `EMB.MULTI` SHALL NOT be used for same-model coalescing
- **AND** `b` and `c` SHALL return the correct embeddings without additional commands

#### Scenario: Mixed-model loaders coalesce into one MULTI

- **WHEN** `Emb[:minilm]["a"]` and `Emb[:bge]["b"]` are created and used in the same scope under a deferred mode
- **THEN** a single command `EMB.MULTI minilm "a" bge "b"` SHALL be sent
- **AND** each value SHALL be the embedding from its own model

#### Scenario: Loaders created after a flush form a new batch

- **WHEN** a batch has already been flushed in the scope under a deferred mode
- **AND** a new loader is then created and used
- **THEN** a new command SHALL be sent containing only the new loader's texts

#### Scenario: Large scope resolves in chunked commands

- **GIVEN** a client configured with `batch_size: 100` and a deferred mode
- **WHEN** a scope defers 250 pairs of one model
- **THEN** the scope resolves via three `EMB` commands with 100, 100, and 50 texts respectively (as shares, possibly executed concurrently in `:batch` mode)
- **AND** results are returned in the deferral order with single-text values as vectors and multi-text values as collections, exactly as with one command

#### Scenario: Chunked failures keep MGET semantics

- **GIVEN** a client configured with `batch_size: 100` and a deferred mode
- **WHEN** a scope defers pairs including an unknown model, spanning two chunks
- **THEN** each mixed-model chunk resolves via `EMB.MULTI` with per-pair `nil` for failed pairs and the loader returns values in deferral order

#### Scenario: batch_size is configurable

- **WHEN** `Emb.configure { |c| c.batch_size = 64 }` is set before clients are created
- **THEN** all clients created afterwards use 64-text chunks
- **AND** an explicit per-client `batch_size:` option overrides the global setting

### Requirement: Cached values within a scope

The gem SHALL cache loaded embeddings within the execution scope under deferred modes, so using the same lazy embedding more than once SHALL NOT trigger additional server commands.

#### Scenario: Repeat use does not re-send

- **WHEN** `loader = Emb[:minilm]["hello"]` is created under a deferred mode and its value is used twice
- **THEN** the pair SHALL be delivered to the server exactly once

#### Scenario: Identical pairs deduplicate

- **WHEN** `a = Emb[:minilm]["dup"]` and `b = Emb[:minilm]["dup"]` are created and both used in the same scope
- **THEN** a single `EMB minilm "dup"` SHALL be sent (single-model scope), carrying one text
- **AND** `a` and `b` SHALL materialize to the same value

### Requirement: Failure handling follows MGET semantics

In mixed-model chunks (delivered as `EMB.MULTI`), a pair whose embedding fails (unknown model or inference error) SHALL materialize as `nil`, mirroring `EMB.MULTI`'s per-pair null behavior, and successful pairs in the same chunk SHALL materialize normally. In single-model chunks (delivered as `EMB`), `EMB` has no per-text partial failures: a model-level failure (unknown model, inference error) surfaces as a whole-command error and follows the fail-closed behavior instead; sibling texts of a failing command are not individually salvageable, which is acceptable because they share the failing model.

#### Scenario: Failed pair yields nil, siblings succeed

- **WHEN** `a = Emb[:minilm]["ok"]` and `b = Emb[:ghost]["nope"]` are created in the same scope under a deferred mode and used
- **AND** the server returns a null for the `ghost` pair
- **THEN** `a` SHALL be an Array of Float
- **AND** `b.nil?` SHALL be `true`

#### Scenario: Checkable without surprising callers

- **WHEN** a failed pair's loader is used
- **THEN** the usage SHALL behave like operating on `nil`, consistent with the existing `Emb.multi` partial-failure behavior

### Requirement: Request-scoped cache clearing

The gem SHALL provide `Emb::Middleware`, a Rack middleware that clears the per-thread batch scope after each request under deferred modes, via `BatchLoader::Executor.clear_current`. Clearing SHALL happen even when the wrapped application raises, and SHALL NOT leak cached values or pending loaders into the next request. In eager mode (`lazy: false`) no per-thread scope exists and the middleware SHALL be inert (requests behave eagerly, nothing to clear).

#### Scenario: Middleware clears the scope after each request

- **WHEN** `Emb::Middleware` wraps an application under `lazy: :multi`
- **AND** the first request creates and uses `Emb[:minilm]["hello"]`
- **AND** the second request creates and uses `Emb[:minilm]["hello"]` again
- **THEN** a new `EMB` command SHALL be sent for the second request (single-model scope)
- **AND** the first request's cached value SHALL NOT be reused

#### Scenario: Scope is cleared when the application raises

- **WHEN** the wrapped application raises an exception under a deferred mode
- **THEN** the per-thread batch scope SHALL still be cleared

#### Scenario: Inert under eager mode

- **WHEN** `Emb::Middleware` wraps an application under the default `lazy: false`
- **THEN** each embed call SHALL send immediately
- **AND** the middleware SHALL perform no scope manipulation

### Requirement: Job-scoped cache clearing

The gem SHALL provide `Emb::JobMiddleware`, a background-job middleware that clears the per-thread batch scope after each job execution under deferred modes, via `BatchLoader::Executor.clear_current`, mirroring the guarantee `Emb::Middleware` provides for Rack requests. Clearing SHALL happen even when the job raises, and each job SHALL start with a fresh batch scope. The middleware SHALL be execution-framework agnostic: it SHALL wrap a job callback and yield to the job body, so the same class can be registered in any job processor (ActiveJob, Sidekiq, Shoryuken, and adapters built on them such as SolidQueue). In eager mode the middleware SHALL be inert.

#### Scenario: Scope is cleared after each job

- **WHEN** `Emb::JobMiddleware` wraps a job body under `lazy: :batch` that creates and uses lazy embeddings
- **THEN** the batch scope SHALL be empty after the job completes
- **AND** a second job on the same thread SHALL start with a fresh scope (the same pair re-sends its command)

#### Scenario: Scope is cleared when the job raises

- **WHEN** `Emb::JobMiddleware` wraps a job body under a deferred mode that raises after creating lazy embeddings
- **THEN** the per-thread batch scope SHALL still be cleared

#### Scenario: Unused loaders are dropped at job end

- **WHEN** a job under a deferred mode creates lazy loaders that are never used
- **THEN** no command SHALL be sent for them
- **AND** they SHALL be dropped when the job ends, not carried into the next job

#### Scenario: Middleware yields to the job body

- **WHEN** `Emb::JobMiddleware` wraps job execution with the framework's middleware signature (e.g. `call(worker, job, queue)` for Sidekiq, `call(worker, queue, sqs_msg, body)` for Shoryuken)
- **THEN** it SHALL invoke the rest of the middleware chain (yield) and the job body regardless of the number of positional arguments
- **AND** it SHALL pass through the job body's return value or exception unchanged

#### Scenario: Inert under eager mode

- **WHEN** `Emb::JobMiddleware` wraps a job body under the default `lazy: false`
- **THEN** the middleware SHALL pass through to the job body with no scope manipulation

### Requirement: Batch failures fail closed

When a batch command fails terminally (timeout after send, connection error after retries are exhausted, or protocol error), the gem SHALL surface the error to the code that forced the batch, SHALL remove every deferred item of that failing share from the scope's pending set, SHALL NOT re-send the failed command on any later resolution attempt, and SHALL NOT re-send shares that already succeeded. Re-resolving an item from a failed share SHALL return an empty array (`[]`) and SHALL NOT perform I/O. New batches created in the same scope afterwards SHALL contain only items deferred after the failure.

#### Scenario: Error surfaces once and pending is cleared

- **WHEN** a scope under a deferred mode defers 6 items and the command for them fails terminally
- **THEN** the forced value SHALL raise the original error
- **AND** the scope's pending set SHALL be empty afterwards

#### Scenario: Retry resolves to [] without I/O

- **WHEN** an item of a failed share is resolved again after the failure
- **THEN** it SHALL return an empty array (`[]`)
- **AND** no command SHALL be sent to the server

#### Scenario: Subsequent batches exclude failed items

- **WHEN** a command has failed terminally in a scope and the scope then defers 2 new items which are forced
- **THEN** the server SHALL receive a single command containing exactly the 2 new pairs
- **AND** none of the previously failed items SHALL be re-sent

#### Scenario: Pending set stays bounded across failures

- **WHEN** several batches fail sequentially in the same scope (with or without retries in between)
- **THEN** the pending set SHALL NOT grow beyond the items currently deferred in the scope

## ADDED Requirements

### Requirement: Lazy mode configuration

The gem SHALL accept a `lazy` mode in its client configuration with exactly three values: `false`, `:multi`, and `:batch`, defaulting to `false`. They are mutually exclusive by construction. With `false` (eager, the default), the standard proxy API (`Emb[:model]["text"]` / `client[:model]["text"]`) SHALL send `EMB` immediately. With `:multi`, proxy embed calls SHALL return lazy batched embeddings that coalesce into one `EMB` command (single-model scope) or one `EMB.MULTI` (mixed-model scope), executed serially. With `:batch`, proxy embed calls SHALL return lazy embeddings whose coalesced chunk shares execute concurrently. Any other value SHALL be rejected at configuration time.

#### Scenario: Default is eager

- **WHEN** `client = Emb.new` is created without a `lazy` option
- **THEN** `client[:minilm]["hello"]` SHALL send `EMB minilm "hello"` at call time and return an Array of Float

#### Scenario: Multi mode defers and coalesces serially

- **WHEN** `client = Emb.new(lazy: :multi)` is created
- **THEN** `client[:minilm]["hello"]` SHALL NOT send a command at call time
- **AND** using the returned value SHALL send `EMB minilm "hello"` and return an Array of Float
- **AND** chunked commands SHALL be executed one at a time (serial)

#### Scenario: Batch mode defers and executes concurrently

- **WHEN** `client = Emb.new(lazy: :batch)` is created and pool-sized connections are available
- **THEN** `client[:minilm]["hello"]` SHALL NOT send a command at call time
- **AND** using the returned value SHALL dispatch chunk shares concurrently and return an Array of Float

#### Scenario: Invalid mode rejected

- **WHEN** `Emb.new(lazy: :eager)` or any value other than `false`/`:multi`/`:batch` is provided
- **THEN** configuration SHALL raise a clear error

### Requirement: Batch mode parallel execution

In `lazy: :batch` mode, the chunk shares of a resolving scope SHALL be dispatched concurrently rather than one after another, and results SHALL be reassembled in deferral order after all shares complete. Concurrency SHALL hold for a single instance (shares run in parallel over that instance's pool connections) and across instances (shares distribute per the `client-multi-instance-distribution` capability).

#### Scenario: Mixed-latency chunks overlap

- **WHEN** a scope under `lazy: :batch` resolves into a slow chunk and a fast chunk on pool-sized connections
- **THEN** both chunks SHALL be dispatched before either completion is awaited
- **AND** the resolving call SHALL complete in approximately the slow chunk's latency, not the sum of both
- **AND** results SHALL be returned in deferral order

#### Scenario: Single-instance concurrency

- **WHEN** `Emb.setup(url: "redis://localhost:6379", lazy: :batch, pool: 4)` is configured and a scope resolves into multiple chunk shares
- **THEN** the shares SHALL execute concurrently over the instance's pool connections
- **AND** all values SHALL materialize correctly in deferral order

#### Scenario: Terminal share failure fails closed

- **WHEN** two shares execute concurrently and one fails terminally after retries
- **THEN** the force SHALL raise the failing share's error
- **AND** the successful share's command SHALL NOT be re-sent on any later resolution
- **AND** the failed share's items SHALL be cleared from the scope's pending set