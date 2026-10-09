## MODIFIED Requirements

### Requirement: Concurrent batch fan-out

In `lazy: :batch` mode, when a scope's deferred work resolves, the client SHALL dispatch its shares — one or more plain `EMB <model> <text>...` commands per model and one command per deferred script call — to instances concurrently rather than serially, SHALL dispatch every share before waiting on any single share's completion, and SHALL reassemble the results in deferral order after all shares complete. Shares SHALL be distributed across instances and their pool connections by the existing round-robin; because the server is stateless any instance MAY answer any share. Fan-out SHALL apply whenever a scope resolves into more than one share, independent of `batch_size`.

#### Scenario: Shares dispatch in parallel across instances

- **WHEN** `lazy: :batch` is configured with 2 urls and a scope defers one text for each of two models
- **THEN** both `EMB` shares SHALL be in flight simultaneously, one to each instance
- **AND** the resolving call SHALL wait for both and return results in deferral order

#### Scenario: Wall time bounded by the slowest share

- **WHEN** a scope resolves into a slow share and a fast share dispatched to different instances
- **THEN** the resolving call SHALL complete in approximately the slow share's latency, not the sum of both
