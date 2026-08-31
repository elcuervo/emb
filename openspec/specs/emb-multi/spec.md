## Purpose

Specifies the `EMB.MULTI` command: embedding several `model text` pairs in one round trip with MGET-style per-pair partial failures, ordered results, and — since the server-inference-isolation change — bounded concurrency so request storms cannot destabilize inference.

## Requirements

### R1: EMB.MULTI command exists

The server SHALL respond to the `EMB.MULTI` command by accepting alternating `model text` pairs and returning an array of embeddings.

#### Scenario: Single pair returns array of one

- **GIVEN** a running server with models `siglip2` and `e5` registered
- **WHEN** client sends `EMB.MULTI siglip2 "hello world"`
- **THEN** response is an array of length 1
- **AND** the single element is a bulk string (the embedding bytes)

#### Scenario: Multiple pairs return ordered array

- **GIVEN** a running server with models `siglip2` and `e5` registered
- **WHEN** client sends `EMB.MULTI siglip2 "text" e5 "query: test"`
- **THEN** response is an array of length 2
- **AND** element 0 is the embedding for `siglip2/"text"`
- **AND** element 1 is the embedding for `e5/"query: test"`

#### Scenario: Odd number of arguments returns error

- **WHEN** client sends `EMB.MULTI siglip2 "text" e5`
- **THEN** response is an error

#### Scenario: Too few arguments returns error

- **WHEN** client sends `EMB.MULTI`
- **THEN** response is an error

### R2: Failures return nil per pair (MGET semantics)

#### Scenario: Unknown model returns nil

- **GIVEN** a running server with model `siglip2` registered
- **WHEN** client sends `EMB.MULTI siglip2 "text" nonexistent "fail"`
- **THEN** response is an array of length 2
- **AND** element 0 is a bulk string (siglip2 embedding)
- **AND** element 1 is a null bulk string

#### Scenario: Inference error returns nil

- **GIVEN** a running server where model `siglip2` exists but will fail on inference
- **WHEN** client sends `EMB.MULTI siglip2 "text"`
- **THEN** response is an array of length 1
- **AND** the single element is a null bulk string

### R3: EMB.STATS counts each pair as one request

#### Scenario: Single MULTI with N pairs increments by N

- **GIVEN** a running server
- **WHEN** client sends `EMB.MULTI siglip2 "a" e5 "b" siglip2 "c"`
- **THEN** `EMB.STATS` shows `total_requests` incremented by 3

### R4: EMB.HELP documents EMB.MULTI

#### Scenario: Help includes MULTI command

- **WHEN** client sends `EMB.HELP`
- **THEN** response includes a line describing `EMB.MULTI` syntax and semantics

### R5: E2E verification with two real models

The server SHALL be verified end-to-end using two distinct downloaded ONNX models to confirm `EMB.MULTI` returns correct embeddings from each model in a single command.

#### Scenario: Two models return correct embeddings in one MULTI

- **GIVEN** two ONNX models downloaded (e.g., `Xenova/all-MiniLM-L6-v2` as `minilm` and `Xenova/multilingual-e5-small` as `e5`)
- **AND** a server config registering both models
- **WHEN** server is started with both models
- **AND** client sends `EMB.MULTI minilm "hello" e5 "query: test"`
- **THEN** response is an array of length 2
- **AND** element 0 is a bulk string with dim matching minilm
- **AND** element 1 is a bulk string with dim matching e5
- **AND** embeddings are byte-identical to the same text via sequential `EMB` calls

#### Scenario: Same model in multiple pairs is batched

- **GIVEN** a server with `minilm` model using smart batching (timeout > 0)
- **WHEN** client sends `EMB.MULTI minilm "a" minilm "b" minilm "c"`
- **THEN** response is an array of length 3
- **AND** all three embeddings have matching dimensions
- **AND** the server's batcher merged the three texts into fewer ONNX runs (observed via latency)

### Requirement: Bounded EMB.MULTI fan-out

The server SHALL cap concurrent pair-processing within a single `EMB.MULTI` to the
model's worker capacity, so request storms cannot spawn unbounded goroutines competing
for inference cores. Per-pair `nil` (MGET) semantics, result ordering, and embedding
correctness SHALL be unchanged.

#### Scenario: Large MULTI does not flood the scheduler

- **GIVEN** a running server with `minilm` registered
- **WHEN** a client sends one `EMB.MULTI` with a large number of pairs (e.g. 400) while
  many concurrent load workers send large `EMB.MULTI` commands for unknown models
- **THEN** the server remains responsive to `PING` and subsequent `EMB` requests
- **AND** concurrent pair-processing is bounded (no unbounded goroutine growth, observed
  via goroutine count / CPU)

#### Scenario: MGET semantics preserved under bounded fan-out

- **GIVEN** a running server with `minilm` registered
- **WHEN** a client sends `EMB.MULTI minilm "a" nonexistent "x" minilm "c"` where the
  "minilm" texts embed successfully and `nonexistent` is unknown
- **THEN** the response SHALL be an array in the same order with element 0 and 2 as bulk
  strings and element 1 as a null bulk string

### Requirement: Storm stability gate

The server SHALL keep inference p99 stable while the request path is under both constant
parse load and a request storm. On the reference machine, under the documented CPU
partition, the constant-load stability ratio (`p99_with_load / p99_idle`) SHALL be ≤ 1.5
and the storm ratio SHALL be ≤ 1.75.

#### Scenario: Constant-load gate passes on the reference machine

- **WHEN** the stability harness runs its constant-load mode under the CPU partition
- **THEN** the reported ratio is ≤ 1.5
- **AND** the gate fails (non-zero exit) otherwise

#### Scenario: Storm gate passes on the reference machine

- **WHEN** the stability harness runs its storm mode (many load workers sending large
  `EMB.MULTI` commands) under the CPU partition
- **THEN** the reported storm ratio is ≤ 1.75
- **AND** the gate fails (non-zero exit) otherwise
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
