# ruby-batch-loading

## Purpose

Specifies lazy client-side batching for the `emb` gem: embedding calls made within the same
execution scope (thread) coalesce into a single `EMB.MULTI` command using the `batch-loader`
gem, with a `batch` configuration option that makes the default proxy API lazy.

## ADDED Requirements

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
delivered to the server as a single `EMB.MULTI` command when the first of them is used.
Embeddings for different models SHALL coalesce into the same command, preserving per-pair
order in the response. Creating loaders SHALL NOT cause I/O; using a value triggers the flush.

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

The gem SHALL accept a `batch` option in its client configuration, defaulting to `false`.
When `true`, the standard proxy API (`Emb[:model]["text"]` / `client[:model]["text"]`)
SHALL return lazy batched embeddings instead of sending commands immediately.
When `false`, the standard proxy API SHALL behave exactly as before this capability.

#### Scenario: Default is eager

- **WHEN** `client = Emb.new` is created without a `batch` option
- **THEN** `client[:minilm]["hello"]` SHALL send `EMB` immediately and return an Array of Float

#### Scenario: Batch mode makes the proxy lazy

- **WHEN** `client = Emb.new(batch: true)` is created
- **THEN** `client[:minilm]["hello"]` SHALL NOT send a command at call time
- **AND** using the returned value SHALL send `EMB.MULTI` and return an Array of Float

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