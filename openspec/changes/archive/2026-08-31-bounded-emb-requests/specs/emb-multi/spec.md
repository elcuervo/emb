# emb-multi Specification

## ADDED Requirements

### Requirement: Pair-count cap truncates EMB.MULTI

The server SHALL process at most the configured `max_pairs` pairs of an `EMB.MULTI` command, truncating the overflow: the reply SHALL have one slot per requested pair, the first `max_pairs` slots per-pair results (or nils for failures within the prefix), the remaining slots `null`. Overflow pairs SHALL NOT be tokenized, inferred, cached, or fanned out. `max_pairs` SHALL default to 4096 when unset and SHALL accept `0` as unlimited.

#### Scenario: Over-cap MULTI truncated before work

- **GIVEN** a running server with `max_pairs` 4096 (default)
- **WHEN** a client sends `EMB.MULTI` with 4097 pairs
- **THEN** the response is an array of 4097 entries
- **AND** the first 4096 entries are the per-pair results
- **AND** the last entry is a null bulk string
- **AND** no pair beyond the cap is tokenized, inferred, or cached

#### Scenario: Acceptable MULTI semantics unchanged

- **GIVEN** a running server with `max_pairs` 4096
- **WHEN** a client sends `EMB.MULTI minilm "a" nonexistent "x" minilm "c"`
- **THEN** the response SHALL be the ordered array with element 0 and 2 as bulk strings and element 1 as a null bulk string (MGET semantics preserved)

#### Scenario: Zero disables the cap

- **GIVEN** a running server with `max_pairs` set to 0
- **WHEN** a client sends `EMB.MULTI` with 50000 pairs
- **THEN** all 50000 pairs SHALL be processed (pre-change behavior)