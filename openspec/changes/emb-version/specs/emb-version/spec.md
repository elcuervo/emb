# emb-version Specification

## Purpose
Exposes the running server build version over the RESP2 protocol so clients and operators can validate which version is executing.

## ADDED Requirements

### Requirement: Version over RESP2
The server SHALL respond to an `EMB.VERSION` command with a bulk string carrying the running build version.

#### Scenario: Returns the build version
- **WHEN** a client sends `EMB.VERSION`
- **THEN** the server SHALL reply with a RESP2 bulk string equal to the `-X main.version` value of the built binary
- **THEN** when no version was injected, the reply SHALL be `dev`

#### Scenario: Wrong argument count
- **WHEN** a client sends `EMB.VERSION` with extra arguments
- **THEN** the server SHALL reply with an error

### Requirement: Unauthenticated version probe
The server SHALL allow `EMB.VERSION` before authentication when a password is configured, so probes can identify the build without credentials.

#### Scenario: Pre-auth on password-protected server
- **WHEN** a password is configured and the client sends `EMB.VERSION` without authenticating
- **THEN** the server SHALL reply with the version (not an auth error)

### Requirement: Command discovery
`EMB.HELP` SHALL document `EMB.VERSION`.

#### Scenario: Help lists the command
- **WHEN** a client sends `EMB.HELP`
- **THEN** the response SHALL include a line for `EMB.VERSION` describing it

### Requirement: Redis INFO compatibility
The server SHALL answer Redis `INFO` / `INFO server` with a `# Server` section whose `redis_version:` line carries the build version, matching what Redis-compatible tooling parses.

#### Scenario: INFO server returns redis_version
- **WHEN** a Redis client sends `INFO server`
- **THEN** the server SHALL reply with a bulk string containing `redis_version:` equal to the build version
- **WHEN** a Redis client sends `INFO` with no arguments
- **THEN** the same server section SHALL be returned