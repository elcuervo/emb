## Purpose

A runnable end-to-end vector application at `examples/kitchensink/` that shows
`emb` doing the half it owns — turning text into vectors — with a real vector
index doing the half it deliberately does not. It exists so a reader can see the
product's contract working instead of reading prose about it.

## ADDED Requirements

### Requirement: A runnable end-to-end example

The repository SHALL carry an example at `examples/kitchensink/` that embeds a
corpus, stores the resulting vectors, and answers ranked queries against them.
It MUST run using only tooling present in the repository's development shell and
dependencies the repository already declares. It MUST NOT require an
installation step beyond the shell, the repository's existing gems, and the
model download its own configuration names.

#### Scenario: A reader runs the example end to end

- **WHEN** a reader follows the commands in `examples/kitchensink/README.md` from the repository's development shell
- **THEN** the corpus is indexed and a query returns ranked results carrying similarity scores

#### Scenario: No dependency is introduced to run it

- **WHEN** the example's dependencies are resolved
- **THEN** they are satisfied by the development shell and the gems the repository already declares, with no new package

### Requirement: Compute and storage are separate processes

The example SHALL obtain embeddings from a running `emb` instance and SHALL
store and search vectors in a vector index served by a separate Redis instance.
The example's own code MUST NOT implement similarity search, and MUST NOT
compute a similarity score to rank results.

#### Scenario: Two servers, one protocol

- **WHEN** the example indexes a corpus and answers a query
- **THEN** embeddings are produced by the `emb` instance and ranking is produced by the Redis instance

#### Scenario: Ranking is not reimplemented

- **WHEN** the example's query path is inspected
- **THEN** it issues the index's own search command and does not score or order candidates itself

### Requirement: Command surface

The example SHALL expose exactly three operations: indexing a set of files,
searching with a query and a result count, and reporting statistics. No
operation MAY require an interactive session, and each MUST be invocable as a
single non-interactive invocation.

#### Scenario: Index

- **WHEN** the example is invoked to index one or more files
- **THEN** the files are chunked, embedded, and stored, and the invocation reports the number of chunks indexed

#### Scenario: Search

- **WHEN** the example is invoked to search with a query
- **THEN** it prints the ranked results for that query

#### Scenario: Stats

- **WHEN** the example is invoked to report statistics
- **THEN** it prints the embedding server's request and cache counters and the index's element count and dimensions

#### Scenario: Missing required argument

- **WHEN** the example is invoked with no operation, with a search that carries no query, or with an index that names no file
- **THEN** it exits non-zero and prints usage on standard error

### Requirement: Indexed text is retrieved from the index

The example SHALL store each chunk's source text alongside its vector such that a
search returns the text without consulting a second store. The example MUST NOT
maintain a parallel key-value structure for chunk payloads.

#### Scenario: Search returns text with no second lookup

- **WHEN** a query returns a ranked result
- **THEN** the result's text and source file accompany it in the same reply, and no additional read is issued to fetch them

#### Scenario: One key holds the corpus

- **WHEN** the example has indexed a corpus
- **THEN** the entire corpus — vectors and payloads — is held under a single index key

### Requirement: Ingestion is content-addressed

Chunk identifiers SHALL be derived from the chunk's content and its source, so
that re-indexing an unchanged corpus produces the same identifiers and the same
element count, and so that an edit replaces only the chunks it touched.

#### Scenario: Re-indexing an unchanged corpus

- **WHEN** the example indexes the same files twice
- **THEN** the index holds the same number of elements after the second run as after the first

#### Scenario: Editing a source file

- **WHEN** one paragraph in an indexed file is edited and the corpus is re-indexed
- **THEN** the elements for unchanged paragraphs are unchanged, and only the edited paragraph's element is replaced

### Requirement: Ingestion is batched

The example SHALL embed a slice of chunks in a single embedding command rather
than issuing one command per chunk, so that the number of client round trips is
bounded by the number of slices rather than by the size of the corpus.

#### Scenario: One embedding command per slice

- **WHEN** a corpus is indexed
- **THEN** the ingest reports one batch per slice of chunks, and the number of batches is the corpus size divided by the batch size, rounded up

#### Scenario: A slice stays within the server's per-command limit

- **WHEN** the example indexes a corpus of any size
- **THEN** each embedding command carries at most the example's configured slice size, which is below the server's default per-command pair limit

### Requirement: A failed chunk does not fail the batch

When an embedding request partially fails, the example SHALL skip the affected
chunks, SHALL continue indexing the remainder of the batch, and SHALL report how
many chunks were skipped. A failed chunk MUST NOT be stored with a null or
placeholder vector.

#### Scenario: Partial failure

- **WHEN** an embedding request returns a null result for some chunks in a batch
- **THEN** the remaining chunks in that batch are indexed, the affected chunks are absent from the index, and the invocation reports the number skipped

#### Scenario: No placeholder vectors

- **WHEN** the index is inspected after a partially failed ingestion
- **THEN** it contains no element whose vector was fabricated to stand in for a failed embedding

### Requirement: Configuration declares the fields the example depends on

The example SHALL carry its own `emb` configuration naming a single embedding
model. That configuration MUST enable model preloading, output normalization,
and caching, because the example's documented behavior depends on each. It MUST
NOT declare a model the example's documented operations do not use.

#### Scenario: Configuration is self-sufficient

- **WHEN** the example is started using its own configuration
- **THEN** the only model it loads is the one its operations use, and no further configuration is required

#### Scenario: Normalization is in effect

- **WHEN** statistics are requested from the example's `emb` instance
- **THEN** the reported model line states that normalization is enabled

#### Scenario: Caching is in effect

- **WHEN** the same query is searched twice
- **THEN** the embedding server's cache hit counter increases and its total request counter does not increase by two

#### Scenario: Query and index agree

- **WHEN** the example embeds a query and a corpus
- **THEN** both use the same model and the same normalization, and the example documents that changing either silently degrades ranking

### Requirement: Documented numbers are reproducible

Every number, score, timing, or count printed in the example's documentation
SHALL have been produced by an actual run of the example. The documentation
MUST NOT advertise an operation, option, or capability the example does not
implement.

#### Scenario: Numbers come from a run

- **WHEN** a number appears in `examples/kitchensink/README.md`
- **THEN** it matches the output of a real invocation of the example

#### Scenario: No advertised capability is missing

- **WHEN** the documentation names an operation or option
- **THEN** the example implements it and accepts it on the command line

### Requirement: One command starts and runs the example

A single documented command SHALL start the example's dependencies, wait until
the embedding server reports itself ready, and run the example. The startup
MUST NOT race the embedding server's readiness.

#### Scenario: Single entry point

- **WHEN** a reader runs the example's documented start command
- **THEN** Redis and the `emb` instance are started and the example runs against them

#### Scenario: Readiness is awaited

- **WHEN** the example's first request is issued
- **THEN** the embedding server has already reported ready, and the example does not fail with a connection error on a cold start

### Requirement: The example is reachable from the task surface

The example SHALL be startable through the repository's standard task runner
under a documented target, so it is discoverable where the repository's other
developer tasks are listed.

#### Scenario: Task runner entry

- **WHEN** a reader lists the repository's documented task targets
- **THEN** the example's target is present and runs the example when invoked
