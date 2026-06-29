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
