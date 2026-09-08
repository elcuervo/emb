# resp3-protocol Specification

## Purpose

RESP protocol negotiation and reply idioms: clients may upgrade a connection to RESP3 via `HELLO`, and every command then replies with RESP3 semantics (typed values, maps, `_` nulls) while RESP2 stays the default. `INFO` remains a bulk string in both protocols.

## ADDED Requirements

### Requirement: HELLO negotiates the protocol version

The server SHALL accept a `HELLO` command followed by a protocol version of `2` or `3`, switching that connection's protocol to the requested version for all subsequent replies on the connection. A bare `HELLO` SHALL report the connection's current protocol version without changing it, returning server metadata (server name, emb version, proto) in the standard Redis HELLO reply shape.

#### Scenario: Upgrade to RESP3

- **GIVEN** a connected client
- **WHEN** the client sends `HELLO 3`
- **THEN** the reply SHALL contain a `proto` field equal to `3` (a RESP3 map; a flat array under RESP2)
- **AND** subsequent replies on that connection SHALL use RESP3 encodings

#### Scenario: Stay on RESP2

- **WHEN** the client sends `HELLO 2`
- **THEN** the reply SHALL contain `proto: 2`
- **AND** subsequent replies SHALL keep RESP2 encodings

#### Scenario: Bare HELLO reports without switching

- **GIVEN** a connection still in RESP2 (default)
- **WHEN** the client sends `HELLO` with no arguments
- **THEN** the reply SHALL report `proto: 2`
- **AND** the connection SHALL remain in RESP2

#### Scenario: Invalid protocol version

- **WHEN** the client sends `HELLO 4`
- **THEN** the server SHALL reply with an error naming the invalid protocol version
- **AND** the connection SHALL remain on its previous protocol

#### Scenario: HELLO respects authentication

- **GIVEN** a server configured with a password and an unauthenticated connection
- **WHEN** the client sends `HELLO 3`
- **THEN** the server SHALL reply with `NOAUTH` unless the password is provided

### Requirement: RESP2 remains the default protocol

A new connection SHALL speak RESP2 until it negotiates otherwise, so existing RESP2 clients keep working with unchanged replies.

#### Scenario: No HELLO means RESP2

- **WHEN** a client connects and sends any command without `HELLO`
- **THEN** all replies SHALL use RESP2 encodings (flat arrays, `$-1` nulls, no typed doubles)

### Requirement: Embedding replies use RESP3 constructs when negotiated

Under RESP3 the embedding reply SHALL use RESP3 constructs as selected by the query's format (see `embedding-reply-format`): typed `,` doubles for `VALUES`, `_` nulls where a REPLY POSITION has no value (e.g., truncated overflow slots under `BLOB`), and bulk strings for binary blobs (legal in both protocols).

#### Scenario: Binary blob shape under RESP3

- **GIVEN** a connection that negotiated RESP3
- **WHEN** the client embeds one text with the default (BLOB) format
- **THEN** the reply SHALL be a bulk string of float32 bytes, identical to the RESP2 reply

#### Scenario: RESP3 null encoding for overflow slots

- **GIVEN** a connection that negotiated RESP3
- **WHEN** an `EMB` command with more texts than the configured cap is sent with the BLOB format
- **THEN** the overflow reply slots SHALL be RESP3 nulls (`_\r\n`, not `$-1`)

### Requirement: Introspection replies become RESP3 maps

When a connection negotiated RESP3, `EMB.INFO`, `EMB.STATS`, `EMB.MODELS`, and `CONFIG GET` SHALL reply with RESP3 maps (alternating key/value pairs under RESP2). `INFO` SHALL remain a bulk string in both protocols.

#### Scenario: EMB.INFO map under RESP3

- **GIVEN** a connection that negotiated RESP3
- **WHEN** the client sends `EMB.INFO minilm`
- **THEN** the reply SHALL be a map whose keys are the same field names as the RESP2 flat-pair reply

#### Scenario: EMB.INFO flat pairs under RESP2

- **GIVEN** a connection that never negotiated RESP3
- **WHEN** the client sends `EMB.INFO minilm`
- **THEN** the reply SHALL be the unchanged flat alternating key/value array

#### Scenario: INFO stays a bulk string under RESP3

- **GIVEN** a connection that negotiated RESP3
- **WHEN** the client sends `INFO`
- **THEN** the reply SHALL be a bulk string in the same `# Section` text format as RESP2

### Requirement: Wire accounting reflects the negotiated protocol

`INFO`'s `total_net_output_bytes` SHALL count the actual bytes written for the negotiated protocol (e.g., `_\r\n` nulls instead of `$-1\r\n`, typed double and map encodings included).

#### Scenario: RESP3 nulls counted at 3 bytes

- **GIVEN** a connection that negotiated RESP3
- **WHEN** a reply containing a null is served
- **THEN** `total_net_output_bytes` SHALL increase by the true RESP3 wire size of that reply (a null contributes 3 bytes, not 5)