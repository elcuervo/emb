# request-size-guardrails Specification

## Purpose

Server-side caps that bound the inference work of a single `EMB`/`EMB.MULTI` command so no client can monopolize a task for unbounded time. Oversized commands are **truncated** to the cap (overflow reply slots become `null` — never a hard failure), with runtime-tunable limits and truncation counters.

## ADDED Requirements

### Requirement: EMB text-count cap truncates

The server SHALL process at most the configured `max_texts` texts of an `EMB` command and SHALL reply with one slot per requested text — the first `max_texts` slots holding embeddings, the remaining slots `null`. Overflow texts SHALL NOT be looked up in the cache, tokenized, inferred, or written to the cache. `max_texts` SHALL default to 4096 when unset and SHALL accept `0` as unlimited.

#### Scenario: Over-cap EMB truncates with null tail

- **GIVEN** a running server with `max_texts` 4096 (default)
- **WHEN** a client sends `EMB minilm <4097 texts>`
- **THEN** the response is an array of 4097 entries
- **AND** the first 4096 entries are bulk strings (embeddings)
- **AND** the last entry is a null bulk string
- **AND** no cache lookup, tokenization, or inference occurs for the 4097th text

#### Scenario: Within-cap EMB unchanged

- **GIVEN** a running server with `max_texts` 4096
- **WHEN** a client sends `EMB minilm <10 texts>`
- **THEN** the response is exactly as before this capability (single bulk for one text, array of bulks for many)

#### Scenario: Zero disables the cap

- **GIVEN** a running server with `max_texts` set to 0
- **WHEN** a client sends `EMB minilm <100000 texts>`
- **THEN** all 100000 texts SHALL be processed (pre-change behavior)

### Requirement: EMB.MULTI pair-count cap truncates

The server SHALL process at most the configured `max_pairs` pairs of an `EMB.MULTI` command and SHALL reply with one slot per requested pair — the first `max_pairs` slots holding embeddings (or per-pair nils for failures within the prefix), the remaining slots `null`. Overflow pairs SHALL NOT be tokenized, inferred, or cached. `max_pairs` SHALL default to 4096 when unset and SHALL accept `0` as unlimited.

#### Scenario: Over-cap MULTI truncates with null tail

- **GIVEN** a running server with `max_pairs` 4096
- **WHEN** a client sends `EMB.MULTI` with 4097 pairs
- **THEN** the response is an array of 4097 entries
- **AND** the first 4096 entries are embeddings for their pairs
- **AND** the last entry is a null bulk string
- **AND** no pair beyond the cap is tokenized, inferred, or cached

#### Scenario: Within-cap MULTI semantics unchanged

- **GIVEN** a running server with `max_pairs` 4096
- **WHEN** a client sends `EMB.MULTI minilm "a" nonexistent "x" minilm "c"`
- **THEN** the response SHALL be the ordered array with element 0 and 2 as bulk strings and element 1 as a null bulk string (MGET semantics preserved)

### Requirement: Truncation counters in EMB.STATS

The server SHALL track and report `truncated_texts` and `truncated_pairs` in `EMB.STATS`, counting overflow texts/pairs dropped by truncation (not the number of commands). Counters SHALL be cumulative since server start and reset on restart.

#### Scenario: Counters reflect truncations

- **GIVEN** a running server with `max_texts` 2 and `max_pairs` 3
- **WHEN** one `EMB` with 4 texts and one `EMB.MULTI` with 5 pairs are sent
- **THEN** `EMB.STATS` reports `truncated_texts` equal to 2 and `truncated_pairs` equal to 2
- **AND** within-cap commands do not increment the counters

### Requirement: Runtime configuration of caps

The server SHALL expose `max_texts` and `max_pairs` through `CONFIG GET`/`CONFIG SET`, the YAML config (`max_texts`, `max_pairs`), and the `-max-texts`/`-max-pairs` flags. Non-integer or negative values SHALL be rejected at config parse (startup error) and by `CONFIG SET` (error reply), and SHALL NOT change the active value.

#### Scenario: CONFIG GET reports caps

- **GIVEN** a running server with default caps
- **WHEN** a client calls `CONFIG GET max_texts`
- **THEN** the response includes `max_texts` with value `4096`

#### Scenario: CONFIG SET adjusts caps live

- **GIVEN** a running server with `max_texts` 4096
- **WHEN** a client calls `CONFIG SET max_texts 1024`
- **THEN** the response is `OK`
- **AND** a subsequent `CONFIG GET max_texts` reports `1024`
- **AND** an `EMB` with 2048 texts now truncates (2048 entries, 1024 bulks + 1024 nulls) while an `EMB` with 512 texts is fully processed

#### Scenario: Invalid values rejected at runtime

- **WHEN** a client calls `CONFIG SET max_texts abc` or `CONFIG SET max_pairs -1`
- **THEN** the response is an error
- **AND** `CONFIG GET max_texts` still reports the previous value

### Requirement: Oversized commands are validated as bounded work

The server SHALL keep the observable work of an oversized command proportional to the cap, not to the command size: the batcher's `requests` and `tokens` counters SHALL reflect only the processed prefix. This is the quantitative validation that a runaway workload cannot saturate a task.

#### Scenario: 100k-pair MULTI contributes at most cap-bound work

- **GIVEN** a running server with `max_pairs` 256 and a model with `max_length` 512
- **AND** no other traffic
- **WHEN** a client sends `EMB.MULTI` with 100000 pairs of near-max-length texts
- **THEN** the server SHALL infer at most 256 pairs
- **AND** `EMB.STATS` SHALL report `total_requests` increased by at most 256
- **AND** `EMB.INFO <model>` SHALL report `tokens` at most 256 × 512 (plus padding), regardless of the 100000-pair command size
- **AND** the response SHALL be an array of 100000 entries with 256 embeddings followed by 99744 nulls
- **AND** `EMB.STATS` SHALL report `truncated_pairs` equal to 99744

#### Scenario: Before the cap, the same workload is unbounded

- **GIVEN** a server with `max_pairs` set to 0 (unlimited, pre-change behavior)
- **WHEN** a client sends the same 100000-pair `EMB.MULTI`
- **THEN** `total_requests` SHALL increase by 100000 and `tokens` SHALL reflect all 100000 pairs — the runaway the cap bounds