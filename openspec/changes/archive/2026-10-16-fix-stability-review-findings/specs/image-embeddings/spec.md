# Spec Delta

## MODIFIED Requirements

### Requirement: Batched image inference

All images of a single `EMB.IMG` (or `EMB.IMGMULTI` pair group) SHALL be preprocessed to the model's fixed tensor shape and inferred in batched session runs bounded by a fixed per-chunk tensor budget, so inference overhead does not grow linearly with the number of images while peak memory stays bounded. Preprocessing MAY be parallel; each chunk SHALL be a single batched call. An output whose batch dimension disagrees with the number of images it was computed from SHALL be a request error naming the mismatch: the server SHALL NOT pool, slice or read past an output tensor it did not get at the batch size it asked for.

#### Scenario: N images, bounded batched inference

- **WHEN** a client sends `EMB.IMG` with N images for one model
- **THEN** the server SHALL execute session runs whose combined batch dimensions are N, using a single run when N fits the per-chunk budget and multiple bounded runs beyond it

#### Scenario: An output whose batch disagrees is refused

- **WHEN** a configured model's output tensor carries fewer (or more) rows than the images in the chunk that was run
- **THEN** the request SHALL fail with an error naming the expected and received batch dimensions
- **AND** the server SHALL NOT panic, and SHALL NOT read rows the output does not have
