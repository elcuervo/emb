## Purpose

Redis-style `CONFIG GET` / `CONFIG SET` over RESP2 so operators can inspect and change the server's runtime-settable parameters without touching the YAML config or restarting.

## ADDED Requirements

### Requirement: CONFIG GET

The server SHALL respond to `CONFIG GET` (no pattern) or `CONFIG GET <glob>` with a RESP2 flat array of `parameter value` bulk-string pairs for parameters whose name matches the glob (`*` matches all; glob syntax = `*` and `?`, like Redis). The response SHALL include both runtime-editable and read-only parameters. Unmatched globs SHALL return an empty array (not an error).

#### Scenario: No pattern returns all parameters

- **WHEN** a client sends `CONFIG GET`
- **THEN** the reply SHALL be an array containing at least the parameters `cache`, `password`, `listen`, `tls_cert`, `tls_key`, `models`, `cache_file`, and `cache_save`, each followed by its current value

#### Scenario: Glob filters parameters

- **WHEN** a client sends `CONFIG GET cache*`
- **THEN** the reply SHALL contain only parameters starting with `cache` (e.g., `cache`, `cache_file`, `cache_save`)

#### Scenario: Unmatched glob

- **WHEN** a client sends `CONFIG GET nonexistent*`
- **THEN** the reply SHALL be an empty array

### Requirement: CONFIG SET for editable parameters

The server SHALL accept `CONFIG SET <param> <value>` and apply the change immediately for runtime-editable parameters: `cache` (resize the cache byte budget immediately, evicting the LRU tail as needed; `auto` and `N%` values recomputed against current system memory at set time), `password` (takes effect for subsequent `AUTH` and the auth gate; already-authenticated connections remain valid), and `cache_file` / `cache_save` (stored; consumed by the next snapshot save — periodic, `EMB.SAVE`, or shutdown — per the cache-snapshot change). The reply SHALL be `OK`.

#### Scenario: Resize cache budget live

- **WHEN** a server cache is at 1GB with 950MB used and `CONFIG SET cache 100MB` is issued
- **THEN** the cache SHALL evict entries until within the new 100MB budget
- **THEN** subsequent `EMB.INFO` / `INFO` SHALL report `cache_max_bytes` near 100MB

#### Scenario: Auto size recomputes at set time

- **WHEN** `CONFIG SET cache auto` is issued
- **THEN** `cache_max_bytes` SHALL become the auto-tuned value for the current machine (≈13% of RAM, no 500MB cap), same as a boot with `cache: "auto"`

#### Scenario: Password change applies to new auth only

- **WHEN** a password is set via `CONFIG SET password hunter2` on a server that had no password
- **THEN** subsequent `AUTH` attempts must use `hunter2`
- **THEN** connections already authenticated before the change SHALL remain valid

#### Scenario: Invalid value rejected

- **WHEN** `CONFIG SET cache nonsense` or `cache 150%` is issued
- **THEN** the server SHALL reply with an error and leave the cache budget unchanged

### Requirement: CONFIG SET rejects read-only parameters

The server SHALL reply with an error for `CONFIG SET` on read-only/restart-only parameters: `listen`, `tls_cert`, `tls_key`, and `models`.

#### Scenario: Read-only parameter refused

- **WHEN** `CONFIG SET listen :9999` is issued
- **THEN** the server SHALL reply with an error naming the parameter as read-only (no listener change occurs)

### Requirement: Config commands require authentication

When a password is configured, `CONFIG GET` and `CONFIG SET` SHALL require prior `AUTH` (unlike `INFO`, which is probe-exempt).

#### Scenario: Pre-auth CONFIG is refused

- **WHEN** a password is configured and a client sends `CONFIG GET` without authenticating
- **THEN** the server SHALL reply with `NOAUTH Authentication required.`

### Requirement: CONFIG documented in EMB.HELP

`EMB.HELP` SHALL document `CONFIG GET` and `CONFIG SET` with their parameter usage.

#### Scenario: Help lists the commands

- **WHEN** a client sends `EMB.HELP`
- **THEN** the response SHALL include lines for `CONFIG GET` and `CONFIG SET` describing the syntax