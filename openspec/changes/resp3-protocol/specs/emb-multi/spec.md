# emb-multi Specification (delta)

## MODIFIED Requirements

### Requirement: EMB.MULTI command exists

The server SHALL respond to the `EMB.MULTI` command by accepting alternating `model text` pairs and returning an array of embeddings. An optional leading `BLOB|VALUES` keyword selects the reply representation (see `embedding-reply-format`); it is recognized only at position 1, case-insensitively, and only when at least one pair follows — otherwise position 1 is the first model name and the format defaults to `BLOB`.

#### Scenario: Single pair returns array of one

- **GIVEN** a running server with models `siglip2` and `e5` registered
- **WHEN** client sends `EMB.MULTI siglip2 "hello world"`
- **THEN** response is an array of length 1
- **AND** the single element is a bulk string (the embedding bytes)

#### Scenario: Leading keyword selects VALUES

- **GIVEN** a running server with models `siglip2` and `e5` registered
- **WHEN** client sends `EMB.MULTI VALUES siglip2 "hello world"`
- **THEN** response is an array of length 1
- **AND** the single element is a VALUES envelope for `siglip2/"hello world"`

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

### Requirement: Failures return nil per pair (MGET semantics)

Under the default BLOB format, a failing pair SHALL return a null bulk string per pair. Under VALUES format, a failing pair SHALL return a null in its position, not an envelope.

#### Scenario: Unknown model returns nil

- **GIVEN** a running server with model `siglip2` registered
- **WHEN** client sends `EMB.MULTI VALUES siglip2 "text" nonexistent "fail"`
- **THEN** response is an array of length 2
- **AND** element 0 is a VALUES envelope (siglip2 embedding)
- **AND** element 1 is a null

#### Scenario: Inference error returns nil

- **GIVEN** a running server where model `siglip2` exists but will fail on inference
- **WHEN** client sends `EMB.MULTI VALUES siglip2 "text"`
- **THEN** response is an array of length 1
- **AND** the single element is a null

## ADDED Requirements

### Requirement: BLOB and VALUES are reserved at position 1

When a leading `BLOB` or `VALUES` keyword is present, the format SHALL apply to the whole command; embedded texts SHALL never be scanned for keywords. The reserved-word model-name validation (see `embedding-reply-format`) SHALL guarantee that an argument at position 1 which is not a recognized keyword is a model name.

#### Scenario: Model named like a keyword is rejected at load

- **WHEN** a model config declares a model named `blob`
- **THEN** server startup SHALL fail (reserved word), so `EMB.MULTI blob ...` can never be ambiguous