# embedding-reply-format

## Purpose

The embedding commands (`EMB`, `EMB.MULTI`) let the caller choose the reply representation with a leading `BLOB|VALUES` keyword, emulating RedisAI's `AI.TENSORGET <key> [META] [BLOB|VALUES]`. `BLOB` (the default) keeps the compact binary wire; `VALUES` returns a self-describing envelope of per-element values (`dtype`, `shape`, `values`) whose value encoding follows the negotiated protocol: RESP3 typed doubles, RESP2 decimal bulk strings.

## Requirements

### Requirement: EMB accepts a leading format keyword

The server SHALL accept `EMB <model> [BLOB|VALUES] <text> [<text>...]`. The keyword is recognized only at position 2 (the argument immediately after the model name), case-insensitively, and only when at least one text argument follows it; otherwise position 2 is the first text and the format defaults to `BLOB`.

#### Scenario: Default is binary blob

- **WHEN** the client sends `EMB minilm "hello world"`
- **THEN** the reply SHALL be the bulk string of float32 bytes (unchanged behavior)

#### Scenario: Explicit BLOB keyword

- **WHEN** the client sends `EMB minilm BLOB "hello world"`
- **THEN** the reply SHALL be identical to the default (bulk string of float32 bytes)

#### Scenario: VALUES keyword with multiple texts

- **WHEN** the client sends `EMB minilm VALUES "hello" "world"`
- **THEN** the reply SHALL be a VALUES envelope covering both texts (see VALUES envelope requirement)

#### Scenario: Text that looks like a keyword

- **WHEN** the client sends `EMB minilm "VALUES"` (single text, no format keyword)
- **THEN** the text `VALUES` SHALL be embedded (default BLOB), because a keyword is only recognized with at least one following text
- **WHEN** the client sends `EMB minilm hello "VALUES" world`
- **THEN** the text `VALUES` in the tail SHALL be embedded, because keyword detection never scans past position 2

#### Scenario: Format keyword as trailing word is safe

- **WHEN** the client sends `EMB minilm "values"` intending the text `values` after other texts
- **THEN** the tail texts SHALL be embedded verbatim (detection is positional, not a sentinel scan)

### Requirement: EMB.MULTI accepts a leading format keyword

The server SHALL accept `EMB.MULTI [BLOB|VALUES] <model> <text> [<model> <text>...]`. The keyword is recognized only at position 1, case-insensitively, and only when at least one pair follows; otherwise position 1 is the first model name and the format defaults to `BLOB`.

#### Scenario: Default MULTI is binary

- **WHEN** the client sends `EMB.MULTI minilm "hello" e5 "query: test"`
- **THEN** the reply SHALL be an array of bulk strings (unchanged behavior)

#### Scenario: Keyword before pairs

- **WHEN** the client sends `EMB.MULTI VALUES minilm "hello" e5 "query: test"`
- **THEN** the reply SHALL be an array of per-pair VALUES envelopes

### Requirement: Reserved model names

The server SHALL reject model names `BLOB` and `VALUES` (case-insensitively) at configuration load, so a keyword at position 1 or 2 can never be mistaken for a model.

#### Scenario: Config with reserved name fails

- **WHEN** a model config declares a model named `values`
- **THEN** server startup SHALL fail with an error naming the reserved word

### Requirement: VALUES replies use a RedisAI envelope

A `VALUES` reply SHALL be a self-describing envelope containing at least `dtype` (`FLOAT`), `shape`, and `values` keys:

- `dtype` SHALL be the bulk string `FLOAT` (embeddings are float32 tensors)
- `shape` SHALL be an array `[m, dim]` where `m` is the number of texts processed and `dim` the model's embedding dimension
- `values` SHALL be a flat array of `m × dim` values, row-major per text

The envelope SHALL be a flat alternating key/value array under RESP2 and a map under RESP3.

#### Scenario: Single text VALUES reply

- **GIVEN** a model with `dim` 3
- **WHEN** the client sends `EMB minilm VALUES "hello"` on a RESP2 connection
- **THEN** the reply SHALL be a flat array `["dtype", "FLOAT", "shape", [1, 3], "values", [...]]`
- **WHEN** the same query is sent on a RESP3 connection
- **THEN** the reply SHALL be a map with keys `dtype`, `shape`, `values`

#### Scenario: Multiple texts are row-major

- **GIVEN** a model with `dim` 2
- **WHEN** the client sends `EMB minilm VALUES "a" "b"` on a RESP3 connection
- **THEN** `shape` SHALL be `[2, 2]`
- **AND** `values` SHALL be `[a0, a1, b0, b1]` (dims of text `a` followed by dims of text `b`)

### Requirement: VALUE encodings follow the negotiated protocol

The individual values SHALL be typed RESP3 doubles (`,<shortest-f64>\r\n`) under RESP3 and decimal bulk strings under RESP2. Each value SHALL be the float64 widening of the float32 dimension, serialized with the standard Redis double representation.

#### Scenario: Typed doubles under RESP3

- **GIVEN** a RESP3 connection
- **WHEN** the client sends `EMB minilm VALUES "hello"`
- **THEN** each element of `values` SHALL be a RESP3 double whose text is the shortest round-trip of the widened float64

#### Scenario: Decimal bulk strings under RESP2

- **GIVEN** a RESP2 connection
- **WHEN** the client sends `EMB minilm VALUES "hello"`
- **THEN** each element of `values` SHALL be a bulk string of the same decimal text

### Requirement: Values preserve float32 fidelity as float64

Widening a float32 dimension to float64 and serializing the shortest round-trip decimal SHALL round-trip back to the same float32 when a client downcasts. No quantization or shortest-float32 formatting SHALL be applied server-side.

#### Scenario: Widened value downcasts to the same float32

- **GIVEN** a float32 dimension whose exact value is not representable in few decimals (e.g., the value `0.1` as float32)
- **WHEN** the server sends its widened float64 text `0.10000000149011612`
- **THEN** a client that parses the text to float64 and downcasts to float32 SHALL recover the original float32 bit pattern

### Requirement: Truncated tails are reflected in shape

When an `EMB` command exceeds the configured max texts in VALUES format, the server SHALL process the head of the list and report `shape[0]` as the number processed; absent trailing texts are implied by the shape rather than encoded as nulls.

#### Scenario: Overflow shortens shape

- **GIVEN** a server whose per-command text cap is `maxTexts`
- **WHEN** the client sends more texts than the cap with VALUES format
- **THEN** the reply SHALL have `shape[m, dim]` with `m` equal to the cap
- **AND** the `values` array SHALL contain `m × dim` elements

### Requirement: EMB.MULTI VALUES replies are per-pair envelopes

An `EMB.MULTI` VALUES reply SHALL be an array with one element per pair: a VALUES envelope including a `model` key (the pair's model), or a null for a failed/truncated pair (MGET semantics preserved).

#### Scenario: Mixed-model VALUES reply

- **WHEN** the client sends `EMB.MULTI VALUES minilm "a" e5 "b"`
- **THEN** the reply SHALL be an array of 2 elements
- **AND** each element SHALL be an envelope with `model` matching the pair's model
- **AND** each `shape` SHALL reflect its own model's dim

#### Scenario: Failure stays null under VALUES

- **WHEN** the client sends `EMB.MULTI VALUES minilm "a" nonexistent "b"`
- **THEN** the element for the failing pair SHALL be a null, not an envelope
