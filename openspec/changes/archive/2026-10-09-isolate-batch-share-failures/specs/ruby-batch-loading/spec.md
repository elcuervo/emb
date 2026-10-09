## MODIFIED Requirements

### Requirement: Batch mode parallel execution

In `lazy: :batch` mode, every share of a resolving scope — each per-model `EMB` chunk and each deferred script call — SHALL be dispatched concurrently rather than one after another, including when the scope fits in a single `batch_size` chunk. Results SHALL be reassembled in deferral order after all shares complete. Concurrency SHALL hold for a single instance (shares run in parallel over that instance's pool connections) and across instances (shares distribute per the `client-multi-instance-distribution` capability). A scope that resolves into exactly one share MAY be sent on the forcing thread without spawning workers. For a scope that resolves into more than one share, a terminal failure of one share SHALL NOT fail the others: the healthy shares SHALL resolve normally, while every item of the failed share SHALL raise that share's `Emb::ServerError` (with the original redis error as `cause`) on each use, without performing I/O. This overrides the raise-on-force and `[]` re-resolution behaviour of "Batch failures fail closed" for multi-share parallel scopes.

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

- **WHEN** two shares execute concurrently and one fails with a non-redis (local) error
- **THEN** the force SHALL raise that original error unchanged after the other shares resolve
- **AND** the successful share's command SHALL NOT be re-sent on any later resolution
- **AND** the scope's pending set SHALL be cleared

#### Scenario: Terminal share failure is isolated

- **GIVEN** two shares execute concurrently and one fails terminally after retries
- **WHEN** either value is forced
- **THEN** the force SHALL NOT raise for the failure of the other share
- **AND** the successful share SHALL resolve normally and its command SHALL NOT be re-sent on any later resolution
- **AND** each item of the failed share SHALL raise that share's `Emb::ServerError` when used, with the original redis error as `cause`
- **AND** re-using an item of the failed share SHALL raise again without performing I/O
- **AND** the failed share's items SHALL be cleared from the scope's pending set
