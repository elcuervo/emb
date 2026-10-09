## MODIFIED Requirements

### Requirement: Per-scope coalescing into EMB.MULTI

All lazy embeddings created in the same execution scope (thread) and batch scope under a deferred mode (`lazy: :multi` or `lazy: :batch`) SHALL be delivered to the server when the first of them is used. Creating loaders SHALL NOT cause I/O; using a value triggers the flush. Chunking by the configured `batch_size` SHALL be unconditional (not triggered by server errors) so a single command never exceeds the server's `max_texts`/`max_pairs` cap and stays within typical client read timeouts.

Under `lazy: :multi`, the scope SHALL be coalesced per client into chunks of at most `batch_size` texts each. A chunk whose items all use one model SHALL be delivered as a single `EMB <model> <text>...` command. A chunk spanning models SHALL be delivered as one `EMB.MULTI <model> <text>...` command, preserving per-pair nil behavior. Chunks are sent one after another.

Under `lazy: :batch`, `EMB.MULTI` SHALL NOT be used. The scope SHALL be grouped per client by model, each model group chunked into `EMB <model> <text>...` shares of at most `batch_size` texts, and the shares dispatched concurrently (per the "Batch mode parallel execution" requirement).

#### Scenario: Same-model loaders coalesce into one MULTI

- **WHEN** `a = Emb[:minilm]["x"]`, `b = Emb[:minilm]["y"]`, and `c = Emb[:minilm]["z"]` are created in the same scope under a deferred mode and `a` is used
- **THEN** a single command `EMB minilm "x" "y" "z"` SHALL be sent to the server
- **AND** `EMB.MULTI` SHALL NOT be used for same-model coalescing
- **AND** `b` and `c` SHALL return the correct embeddings without additional commands

#### Scenario: Mixed-model loaders coalesce into one MULTI

- **WHEN** `Emb[:minilm]["a"]` and `Emb[:bge]["b"]` are created and used in the same scope under `lazy: :multi`
- **THEN** a single command `EMB.MULTI minilm "a" bge "b"` SHALL be sent
- **AND** each value SHALL be the embedding from its own model

#### Scenario: Mixed-model loaders split per model under batch mode

- **WHEN** `Emb[:minilm]["a"]`, `Emb[:bge]["b"]`, and `Emb[:minilm]["c"]` are created and used in the same scope under `lazy: :batch`
- **THEN** exactly two commands SHALL be sent: `EMB minilm "a" "c"` and `EMB bge "b"`
- **AND** `EMB.MULTI` SHALL NOT be sent
- **AND** each value SHALL be the embedding from its own model, in deferral order

#### Scenario: Loaders created after a flush form a new batch

- **WHEN** a batch has already been flushed in the scope under a deferred mode
- **AND** a new loader is then created and used
- **THEN** a new command SHALL be sent containing only the new loader's texts

#### Scenario: Large scope resolves in chunked commands

- **GIVEN** a client configured with `batch_size: 100` and a deferred mode
- **WHEN** a scope defers 250 pairs of one model
- **THEN** the scope resolves via three `EMB` commands with 100, 100, and 50 texts respectively (executed concurrently in `:batch` mode)
- **AND** results are returned in the deferral order with single-text values as vectors and multi-text values as collections, exactly as with one command

#### Scenario: Chunked failures keep MGET semantics

- **GIVEN** a client configured with `batch_size: 100` and `lazy: :multi`
- **WHEN** a scope defers pairs including an unknown model, spanning two chunks
- **THEN** each mixed-model chunk resolves via `EMB.MULTI` with per-pair `nil` for failed pairs and the loader returns values in deferral order

#### Scenario: Unknown model share fails alone under batch mode

- **WHEN** a scope under `lazy: :batch` defers `Emb[:minilm]["a"]` and `Emb[:nope]["b"]`
- **THEN** the `EMB nope "b"` share's error SHALL fail the force per "Batch failures fail closed"
- **AND** the `EMB minilm "a"` share SHALL have been sent concurrently and SHALL NOT be re-sent on a later resolution

#### Scenario: batch_size is configurable

- **WHEN** `Emb.configure { |c| c.batch_size = 64 }` is set before clients are created
- **THEN** all clients created afterwards use 64-text chunks
- **AND** an explicit per-client `batch_size:` option overrides the global setting

### Requirement: Lazy mode configuration

The gem SHALL accept a `lazy` mode in its client configuration with exactly three values: `false`, `:multi`, and `:batch`, defaulting to `false`. They are mutually exclusive by construction. With `false` (eager, the default), the standard proxy API (`Emb[:model]["text"]` / `client[:model]["text"]`) SHALL send `EMB` immediately. With `:multi`, proxy embed calls SHALL return lazy batched embeddings that coalesce into one `EMB` command (single-model scope) or one `EMB.MULTI` (mixed-model scope), executed serially. With `:batch`, proxy embed calls SHALL return lazy embeddings that resolve through independent plain `EMB` shares (one or more per model) executed concurrently; `:batch` SHALL NOT send `EMB.MULTI`. Any other value SHALL be rejected at configuration time.

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
- **AND** using the returned value SHALL dispatch per-model `EMB` shares concurrently and return an Array of Float

#### Scenario: Invalid mode rejected

- **WHEN** `Emb.new(lazy: :eager)` or any value other than `false`/`:multi`/`:batch` is provided
- **THEN** configuration SHALL raise a clear error

### Requirement: Batch mode parallel execution

In `lazy: :batch` mode, every share of a resolving scope — each per-model `EMB` chunk and each deferred script call — SHALL be dispatched concurrently rather than one after another, including when the scope fits in a single `batch_size` chunk. Results SHALL be reassembled in deferral order after all shares complete. Concurrency SHALL hold for a single instance (shares run in parallel over that instance's pool connections) and across instances (shares distribute per the `client-multi-instance-distribution` capability). A scope that resolves into exactly one share MAY be sent on the forcing thread without spawning workers.

#### Scenario: Small mixed-model scope runs in parallel

- **GIVEN** `lazy: :batch` with the default `batch_size` and pool-sized connections
- **WHEN** a scope defers one `siglip2` text and one `hyperclusters` text and either value is used
- **THEN** `EMB siglip2 <text>` and `EMB hyperclusters <text>` SHALL both be in flight before either reply is awaited
- **AND** the force SHALL complete in approximately the slower command's latency, not the sum of both
- **AND** using the other value afterwards SHALL NOT send another command

#### Scenario: Mixed-latency chunks overlap

- **WHEN** a scope under `lazy: :batch` resolves into a slow share and a fast share on pool-sized connections
- **THEN** both shares SHALL be dispatched before either completion is awaited
- **AND** the resolving call SHALL complete in approximately the slow share's latency, not the sum of both
- **AND** results SHALL be returned in deferral order

#### Scenario: Single-instance concurrency

- **WHEN** `Emb.setup(url: "redis://localhost:6379", lazy: :batch, pool: 4)` is configured and a scope resolves into multiple shares
- **THEN** the shares SHALL execute concurrently over the instance's pool connections
- **AND** all values SHALL materialize correctly in deferral order

#### Scenario: Terminal share failure fails closed

- **WHEN** two shares execute concurrently and one fails terminally after retries
- **THEN** the force SHALL raise the failing share's error
- **AND** the successful share's command SHALL NOT be re-sent on any later resolution
- **AND** the failed share's items SHALL be cleared from the scope's pending set

## ADDED Requirements

### Requirement: Deferred script evaluation

`eval` and `evalsha` SHALL honor the client's `lazy` mode: eager under `false`, deferred under `:multi` and `:batch`. A deferred call SHALL join the same scope as deferred embeddings, send its own `EMB.EVAL`/`EMB.EVSHA` command (never coalesced), and apply `decode:` on resolution. Under `:multi` the scope's commands SHALL be sent serially; under `:batch` each call SHALL be its own share dispatched concurrently with the scope's `EMB` shares. Script errors SHALL fail closed like embed shares.

#### Scenario: Eager mode unchanged

- **WHEN** `client = Emb.new` (lazy `false`) calls `client.evalsha(:gliner, sha, ["text"], ["places"])`
- **THEN** `EMB.EVSHA gliner <sha> 1 "text" "places"` SHALL be sent at call time and the parsed reply returned

#### Scenario: Batch mode runs scripts in parallel with embeddings

- **GIVEN** `client = Emb.new(lazy: :batch)` with pool-sized connections
- **WHEN** a scope defers `client[:siglip2]["q"]` and `client.evalsha(:gliner, sha, ["q"], ["places"])` and one value is used
- **THEN** `EMB siglip2 "q"` and `EMB.EVSHA gliner <sha> 1 "q" "places"` SHALL both be in flight before either reply is awaited
- **AND** neither SHALL be wrapped in `EMB.MULTI`
- **AND** using the other value SHALL NOT send another command

#### Scenario: Two script calls stay independent

- **WHEN** a scope under `lazy: :batch` defers two `evalsha` calls with different args
- **THEN** two `EMB.EVSHA` commands SHALL be dispatched concurrently, one per call, each reply mapped to its own value

#### Scenario: Multi mode sends scripts serially

- **WHEN** a scope under `lazy: :multi` defers `client[:minilm]["a"]` and `client.eval(:minilm, script, ["a"])` and one value is used
- **THEN** `EMB minilm "a"` and `EMB.EVAL minilm <script> 1 "a"` SHALL be sent one after another on the forcing thread

#### Scenario: Decode applies at resolution

- **WHEN** a deferred `evalsha(..., decode: :f32)` resolves to a string bulk reply
- **THEN** the value SHALL be the decoded Array of Float, identical to the eager result
