## MODIFIED Requirements

### Requirement: Eviction

When the cache reaches its byte budget, the least recently used entries SHALL be evicted. After every successful insert or replacement with a positive configured budget, accounted live entry bytes SHALL NOT exceed that budget. Entry admission SHALL include the key, value, and existing per-entry accounting overhead. Values too large to fit by themselves SHALL be rejected before copying their payload or evicting any existing entry. Rejected writes SHALL leave values, LRU order, byte accounting, counters, and mutation generation unchanged. Existing non-positive internal budget behavior SHALL remain unchanged.

#### Scenario: Eviction on insert

- **GIVEN** the cache is at its byte budget
- **WHEN** a new text is embedded and its entry fits the budget by itself
- **THEN** the necessary LRU entries SHALL be evicted before the new entry is stored
- **AND** the eviction counter SHALL be incremented for each removed entry

#### Scenario: Oversized insert preserves useful entries

- **GIVEN** a positive cache budget and existing entries
- **WHEN** a new key and value exceed that budget including accounting overhead
- **THEN** the write SHALL be skipped before allocating an owned payload copy
- **AND** all existing entries, order, accounting, counters, and generation SHALL remain unchanged

#### Scenario: Oversized replacement preserves the old value

- **GIVEN** a cached key under a positive budget
- **WHEN** its proposed replacement cannot fit the budget by itself
- **THEN** the original value and recency SHALL remain unchanged
- **AND** no eviction, accounting, counter, or generation change SHALL occur

#### Scenario: Growing replacement evicts other least-recent entries

- **GIVEN** a cached key whose replacement fits the budget by itself but exceeds the remaining free space
- **WHEN** the replacement is stored
- **THEN** the replacement SHALL become most recently used
- **AND** other least-recent entries SHALL be evicted until live accounted bytes fit the budget
- **AND** byte, per-model, eviction, and mutation-generation accounting SHALL reflect the actual mutations

#### Scenario: Exact-fit and shrinking writes

- **WHEN** a write makes accounted bytes equal the positive budget, or replaces a value with a smaller one without exceeding the budget
- **THEN** the write SHALL succeed without unnecessary eviction
- **AND** previously captured snapshot values SHALL remain immutable
