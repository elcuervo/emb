## ADDED Requirements

### Requirement: Maximum command byte size

The server SHALL enforce a configurable maximum byte size for a single command and SHALL reject a command that exceeds it without decoding or inferring any of it. The bound SHALL apply to the bytes buffered for one command, so a client cannot make the server allocate unbounded memory by sending an oversized bulk value. The server SHALL report the rejection as an error and SHALL NOT count a rejected command as processed work.

#### Scenario: Oversized command is rejected

- **WHEN** a client sends a command whose total buffered size exceeds `max_command_bytes`
- **THEN** the server rejects it with an error and performs no decode or inference

#### Scenario: Oversized declared bulk is refused before buffering

- **WHEN** a bulk header declares a length greater than the configured maximum
- **THEN** the server refuses the command before reading the declared payload into memory

#### Scenario: Normal commands are unaffected

- **WHEN** a command is within the configured maximum
- **THEN** it is processed exactly as before

### Requirement: Image payload byte limits are configurable

The server SHALL expose the per-image byte cap and the command byte cap through the YAML config and `CONFIG GET`/`CONFIG SET` alongside the existing `max_texts` and `max_pairs` limits, and SHALL reject non-integer or negative values at config parse and via `CONFIG SET`.

#### Scenario: Runtime inspection

- **WHEN** a client sends `CONFIG GET max_command_bytes`
- **THEN** the reply contains the active value

#### Scenario: Invalid value rejected

- **WHEN** `CONFIG SET max_command_bytes` is given a negative or non-integer value
- **THEN** the server replies with an error and leaves the active value unchanged
