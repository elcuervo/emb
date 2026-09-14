## ADDED Requirements

### Requirement: Image command reply formats

`EMB.IMG` SHALL accept the `BLOB|VALUES` keyword at position 2 (immediately after the model), and `EMB.IMGMULTI` SHALL accept it at position 1, with the same case-insensitive, follows-at-least-one-item recognition rule as `EMB` and `EMB.MULTI` respectively. Under `VALUES`, an `EMB.IMG` reply SHALL be a self-describing envelope with `dtype` (`FLOAT`), `shape` `[m, dim]` (`m` = images processed), and a flat row-major `values` array, exactly as for text embeddings; an `EMB.IMGMULTI` reply SHALL be an array with one `VALUES` envelope (including a `model` key) per pair, or a null for a failed/truncated pair. Image embeddings SHALL be float32 vectors of the model's configured dimension and SHALL use the same RESP2/RESP3 encoding as text embeddings.

#### Scenario: EMB.IMG VALUES envelope

- **WHEN** a client sends `EMB.IMG <model> VALUES <bytes>`
- **THEN** the reply is a VALUES envelope whose `shape` is `[1, dim]` and whose `dtype` is `FLOAT`

#### Scenario: EMB.IMGMULTI VALUES per pair

- **WHEN** a client sends `EMB.IMGMULTI VALUES <model> <bytes> <model> <bytes>`
- **THEN** the reply is an array of two VALUES envelopes, each carrying its pair's `model`

#### Scenario: Failed pair is null under VALUES

- **WHEN** an `EMB.IMGMULTI VALUES` pair fails
- **THEN** its array position is null rather than an envelope
