## Purpose

Ensure `EMB.INFO` RESP responses have the correct array element count
matching the actual number of elements returned, preventing client-side
timeouts and partial reads.

## Requirements

### Requirement: EMB.INFO returns the correct number of array elements

The server SHALL write a RESP array count that exactly matches the number of elements
returned by `EMB.INFO`, regardless of whether caching is enabled or disabled.

#### Scenario: INFO without cache returns 22 elements

- **WHEN** the server has no cache configured
- **AND** `EMB.INFO minilm` is called
- **THEN** the response SHALL have exactly 22 array elements (11 key-value pairs)
- **THEN** all expected keys SHALL be present: `dim`, `max_length`, `workers`, `requests`, `avg_latency_us`, `tokens`, `errors`, `pooling`, `normalize`, `batching_timeout_ms`, `batching_max_batch`

#### Scenario: INFO with cache returns 36 elements

- **WHEN** the server has a cache configured
- **AND** `EMB.INFO minilm` is called
- **THEN** the response SHALL have exactly 36 array elements (18 key-value pairs)
- **THEN** all non-cache keys SHALL be present
- **THEN** cache keys SHALL be present: `cache_hits`, `cache_misses`, `cache_hit_rate`, `cache_evictions`, `cache_entries`, `cache_max_bytes`, `cache_memory_bytes`

#### Scenario: Go test parses full RESP and validates count

- **WHEN** a Go test calls `EMB.INFO` on a server without cache
- **THEN** the test SHALL parse the RESP response and verify the declared array count equals `22`
- **THEN** the test SHALL parse all 22 elements without error
- **WHEN** `EMB.INFO` is called on a server with cache
- **THEN** the test SHALL parse the RESP response and verify the declared array count equals `36`
- **THEN** the test SHALL parse all 36 elements without error

#### Scenario: Client can parse INFO without timeout

- **WHEN** a Redis client calls `EMB.INFO minilm`
- **THEN** the client SHALL receive the complete response and return it as key-value pairs
- **THEN** the client SHALL NOT timeout waiting for additional array elements
