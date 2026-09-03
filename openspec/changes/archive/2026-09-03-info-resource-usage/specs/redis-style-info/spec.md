## MODIFIED Requirements

### Requirement: Sectioned INFO command

The server SHALL respond to `INFO` (and `INFO <section>...`) with a RESP2 bulk string in Redis section format: lines grouped under `# SectionName` headers, keys as `key:value` with `\r\n` line endings.

#### Scenario: No arguments returns all sections

- **WHEN** a Redis client sends `INFO` with no arguments
- **THEN** the reply SHALL be a bulk string containing at least `# Server`, `# Cache`, `# Keyspace`, `# Stats`, `# Memory`, `# CPU`, and `# Clients` sections in that order

#### Scenario: Section argument filters

- **WHEN** a Redis client sends `INFO server`
- **THEN** the reply SHALL contain only the `# Server` section

#### Scenario: Multiple section arguments

- **WHEN** a Redis client sends `INFO cache stats`
- **THEN** the reply SHALL contain only the `# Cache` and `# Stats` sections

#### Scenario: Unknown section name

- **WHEN** a Redis client sends `INFO nonexistent`
- **THEN** the reply SHALL be a bulk string with no sections (empty body)