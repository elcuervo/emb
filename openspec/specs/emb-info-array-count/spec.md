## Purpose

Ensure `EMB.INFO` RESP responses have the correct array element count
matching the actual number of elements returned, preventing client-side
timeouts and partial reads.

## Requirements

### Requirement: EMB.INFO's declared array count matches what it emits

The server SHALL write a RESP array count that exactly matches the number of elements `EMB.INFO` returns, whether or not a cache is configured, and SHALL emit each configured group in full. A test that pins the reply's shape SHALL compare the declared count against the elements it parsed, rather than against a hard-coded total.

#### Scenario: Declared count matches what was emitted

- **WHEN** `EMB.INFO <model>` is called on a server with no cache configured
- **THEN** the declared RESP array count SHALL equal the number of elements that follow it
- **AND** the model's own fields SHALL all be present, so no pair is cut short

#### Scenario: The cache group is complete

- **WHEN** `EMB.INFO <model>` is called on a server with a cache configured
- **THEN** the declared count SHALL equal the emitted element count
- **AND** the cache group SHALL be present in full (`cache_hits`, `cache_misses`, `cache_hit_rate`, `cache_evictions`, `cache_entries`, `cache_max_bytes`, `cache_memory_bytes`)

#### Scenario: Go test parses full RESP and validates count

- **WHEN** a Go test calls `EMB.INFO` on a server without a cache, and again on one with a cache
- **THEN** the test SHALL parse every element the reply declares and verify the declared count equals the number of elements parsed
- **AND** the test SHALL NOT assert a hard-coded element total that a new field would invalidate

#### Scenario: Client can parse INFO without timeout

- **WHEN** a Redis client calls `EMB.INFO minilm`
- **THEN** the client SHALL receive the complete response and return it as key-value pairs
- **THEN** the client SHALL NOT timeout waiting for additional array elements

#### Scenario: A new field does not re-stale the check

- **WHEN** a field is added to the model's own group or to the cache group
- **THEN** the count check SHALL still pass, because it asserts equality rather than a remembered total
