# ruby-batch-loading Specification

## MODIFIED Requirements

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