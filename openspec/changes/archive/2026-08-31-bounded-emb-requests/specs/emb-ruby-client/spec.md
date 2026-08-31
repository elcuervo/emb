# emb-ruby-client Specification

## MODIFIED Requirements

### Requirement: Multi-model batch

The gem SHALL support batch multi-model embedding via a block syntax, on both module level and instance level. The composed `EMB.MULTI` SHALL be split into commands of at most the configured `batch_size` pairs (default 512), preserving result ordering and per-pair nil behavior across chunks.

#### Scenario: Multi-embed block (module level)

- **WHEN** `Emb.multi { |m| m[:minilm]["hello"]; m[:bge]["world"] }` is called with a default batch_size of 512
- **THEN** it SHALL send `EMB.MULTI minilm "hello" bge "world"` in a single command (2 pairs ≤ 512)
- **THEN** each result SHALL be unpacked from float32 binary to an Array of Float

#### Scenario: Multi-embed block (instance)

- **WHEN** `client.multi { |m| m[:minilm]["hello"]; m[:bge]["world"] }` is called
- **THEN** it SHALL return unpacked float arrays, same as the module-level call

#### Scenario: Oversized block chunks and reassembles

- **WHEN** `client.multi` with `batch_size` 100 collects 250 pairs across models
- **THEN** it SHALL send three `EMB.MULTI` commands (100, 100, 50 pairs)
- **AND** the returned array SHALL be in collection order across all chunks, with failed pairs as `nil`

#### Scenario: batch_size applies globally and per client

- **WHEN** `Emb.configure { |c| c.batch_size = 64 }` is set
- **THEN** default and `Emb.new` clients SHALL use 64-pair chunks
- **AND** `Emb.new(batch_size: 32)` SHALL override with 32