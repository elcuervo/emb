## ADDED Requirements

### Requirement: Cache stores owned copies of values

`Set` SHALL store an independent copy of the value bytes rather than a caller-owned slice. A stored entry SHALL NOT alias a shared backing buffer (such as a batch-wide embedding buffer whose per-row values are sub-slices), so retaining one small entry never pins a much larger buffer. The byte accounting reported by cache stats SHALL reflect the bytes the cache actually retains.

#### Scenario: Mutating the source does not change the cached value

- **WHEN** a value slice is stored with `Set` and the caller then mutates the same underlying array
- **THEN** a subsequent `Get` SHALL return the bytes as stored, not the mutated bytes

#### Scenario: A single cached row does not retain its batch

- **GIVEN** a batch inference that produced many rows in one backing buffer
- **WHEN** exactly one row is stored in the cache
- **THEN** the memory the cache retains SHALL be proportional to that row, not to the whole batch
