## MODIFIED Requirements

### Requirement: CONFIG GET

The server SHALL respond to `CONFIG GET` (no pattern) or `CONFIG GET <glob>` with a RESP2 flat array of `parameter value` bulk-string pairs for parameters whose name matches the glob (`*` matches all; glob syntax = `*` and `?`, like Redis). The response SHALL include both runtime-editable and read-only parameters. Each parameter's reported value SHALL be its current effective value, so a value changed by `CONFIG SET` (including `cache`) is echoed by a later `CONFIG GET`. Unmatched globs SHALL return an empty array (not an error).

#### Scenario: No pattern returns all parameters

- **WHEN** a client sends `CONFIG GET`
- **THEN** the reply SHALL be an array containing at least the parameters `cache`, `password`, `listen`, `tls_cert`, `tls_key`, `models`, `cache_file`, and `cache_save`, each followed by its current value

#### Scenario: Glob filters parameters

- **WHEN** a client sends `CONFIG GET cache*`
- **THEN** the reply SHALL contain only parameters starting with `cache` (e.g., `cache`, `cache_file`, `cache_save`)

#### Scenario: Unmatched glob

- **WHEN** a client sends `CONFIG GET nonexistent*`
- **THEN** the reply SHALL be an empty array

#### Scenario: Runtime-set value is echoed

- **WHEN** `CONFIG SET cache 128mb` is issued against a server whose cache was configured at boot with a different value
- **THEN** a subsequent `CONFIG GET cache` SHALL report `128mb`
- **AND** the effective budget reported by `EMB.INFO` SHALL match that value

## ADDED Requirements

### Requirement: Runtime-editable parameters are concurrency-safe

Changing a runtime-editable parameter with `CONFIG SET` while commands are being served SHALL be free of data races: a concurrent read of `max_texts`, `max_pairs`, `max_images`, `max_image_bytes`, `max_image_pixels`, or `max_command_bytes` by a request handler SHALL be synchronized with the setter. The race detector SHALL report no data race when `CONFIG SET` and inference commands run concurrently.

#### Scenario: CONFIG SET races no in-flight request

- **GIVEN** a server built with the race detector
- **WHEN** one client repeatedly issues `CONFIG SET max_texts <n>` and `CONFIG SET max_command_bytes <n>` while other clients issue `EMB` and `EMB.MULTI` commands
- **THEN** the process SHALL report no data race

#### Scenario: Single-command validation is consistent

- **WHEN** a request is validated against a cap that a concurrent `CONFIG SET` changes
- **THEN** the request SHALL observe a single, whole value of that cap
