## ADDED Requirements

### Requirement: EMB.IMGMULTI cross-model image embedding

The server SHALL respond to `EMB.IMGMULTI [BLOB|VALUES] <model> <bytes> [<model> <bytes>...]` by accepting alternating `model image-bytes` pairs, where each `<bytes>` is the raw encoded content of an image sent as a binary-safe RESP bulk, and returning an array with one element per pair. An optional leading `BLOB|VALUES` keyword SHALL be recognized only at position 1, case-insensitively, and only when at least one pair follows. A failing pair SHALL return a null in its position (MGET semantics) without failing the command, and results SHALL preserve request order. The server SHALL NOT fetch remote URLs. Each pair SHALL be counted as one request in `EMB.STATS`, and `max_pairs` truncation SHALL apply exactly as for `EMB.MULTI`.

#### Scenario: Two models in one command

- **WHEN** a client sends `EMB.IMGMULTI clip <a.jpg bytes> siglip2 <b.jpg bytes>`
- **THEN** the reply is an array of two embeddings, the first from `clip` and the second from `siglip2`, in order

#### Scenario: One pair fails, others succeed

- **WHEN** one pair names an unknown model or its image bytes fail to decode
- **THEN** that element is null and the remaining pairs still return embeddings

#### Scenario: max_pairs truncation

- **WHEN** more pairs are supplied than `max_pairs`
- **THEN** only the first `max_pairs` pairs are processed and the overflow reply slots are null

#### Scenario: Format keyword position

- **WHEN** a client sends `EMB.IMGMULTI VALUES clip <a.jpg bytes>`
- **THEN** `VALUES` is interpreted as the reply format and the following argument is the first model
