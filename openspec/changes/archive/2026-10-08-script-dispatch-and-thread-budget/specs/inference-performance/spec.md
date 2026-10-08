## ADDED Requirements

### Requirement: Work-conserving dispatch for unbatched pools

Unbatched embedding pools and image session pools SHALL dispatch each request to a free worker or session when one exists, rather than to a fixed round-robin index. Embeddings SHALL be unchanged.

#### Scenario: Free worker serves the request

- **GIVEN** an unbatched pool with two workers, one busy with a long request
- **WHEN** a new request arrives
- **THEN** it SHALL be served by the free worker without waiting for the busy one

#### Scenario: Embeddings unchanged

- **WHEN** the same texts are embedded before and after the dispatch change
- **THEN** the returned embeddings SHALL be byte-identical

### Requirement: Thread oversubscription warning

At boot, the server SHALL log a warning when the total intra-op threads across all loaded sessions (embedding, image and script) exceed the available cores. The warning SHALL name the contributing models with their session and thread counts. The configuration SHALL NOT be changed.

#### Scenario: Oversubscribed configuration warns

- **WHEN** a model is configured with `script_workers: 4` and `intra_op_threads: 8` on a 10-core host
- **THEN** the boot log SHALL contain a warning naming that model, 4 sessions and 8 threads
