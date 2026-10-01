## ADDED Requirements

### Requirement: Readiness predicate reports the server state

The gem SHALL expose `ready?` as a boolean predicate on the client and module: it SHALL return `true` only when `EMB.READY` answers `OK`, and `false` when the server answers with an error (loading, draining, no models) or when the server cannot be reached. Neither `ready?` nor `ready` SHALL raise for an unreachable server. The companion `ready` accessor SHALL return the server's `OK` text on success and its error text on failure — including connection failures — so it is always a status string.

#### Scenario: Ready server

- **WHEN** `client.ready?` is called against a server that answers `EMB.READY` with `OK`
- **THEN** it SHALL return `true`

#### Scenario: Server reports not ready

- **WHEN** `EMB.READY` answers with an error (for example `ERR loading`)
- **THEN** `client.ready?` SHALL return `false`
- **AND** `client.ready` SHALL return the server's error message string

#### Scenario: Server unreachable

- **WHEN** the server refuses the connection
- **THEN** `client.ready?` SHALL return `false`
- **AND** `client.ready` SHALL return the connection error text
- **AND** neither SHALL raise

### Requirement: Embed replies tolerate null slots

The gem SHALL map a null (nil) reply slot on an embed response to Ruby `nil` rather than raising, so a reply that carries nulls for failed or truncated positions is returned as-is. This SHALL apply to the eager binary embed path as well as the image path.

#### Scenario: Null slot in a binary embed reply

- **WHEN** an `EMB` or `EMB.IMG` reply contains a null slot among the vector slots
- **THEN** the gem SHALL return `nil` for that position and unpacked vectors for the others
- **AND** it SHALL NOT raise a `NoMethodError`
