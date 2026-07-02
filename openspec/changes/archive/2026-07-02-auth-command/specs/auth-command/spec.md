## ADDED Requirements

### Requirement: Password configuration
The server SHALL accept an optional password via the YAML `password` field or the `-password` CLI flag. When absent or empty, no authentication is enforced.

### Requirement: AUTH command
The server SHALL respond to the `AUTH` command per Redis `requirepass` semantics.

#### Scenario: AUTH with no password configured
- **WHEN** the server has no password configured and a client sends `AUTH <any>`
- **THEN** the server responds with `-ERR Client sent AUTH, but no password is set`

#### Scenario: AUTH with wrong password
- **WHEN** the server has a password configured and a client sends `AUTH <wrong>`
- **THEN** the server responds with `-ERR invalid password`

#### Scenario: AUTH with correct password
- **WHEN** the server has a password configured and a client sends `AUTH <correct>`
- **THEN** the server responds with `+OK`

#### Scenario: Double AUTH
- **WHEN** a client sends `AUTH <correct>` twice
- **THEN** both requests return `+OK`

### Requirement: Auth enforcement
When a password is configured, the server SHALL reject non-exempt commands on unauthenticated connections.

#### Scenario: Command before auth
- **WHEN** a client sends any command (except `PING` or `AUTH`) before authenticating
- **THEN** the server responds with `-NOAUTH Authentication required.`

#### Scenario: PING before auth
- **WHEN** a client sends `PING` before authenticating
- **THEN** the server responds with `+PONG`

#### Scenario: Commands work after auth
- **WHEN** a client sends `AUTH <correct>` followed by any command
- **THEN** the command executes normally

### Requirement: No auth mode
When no password is configured, the server SHALL behave identically to today — no auth checks, no AUTH command registration effect.

#### Scenario: No password, regular commands
- **WHEN** the server has no password configured
- **THEN** all commands work without authentication
