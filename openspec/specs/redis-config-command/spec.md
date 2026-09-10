## Purpose

Redis-style `CONFIG GET` / `CONFIG SET` over RESP2 so operators can inspect and change the server's runtime-settable parameters without touching the YAML config or restarting.

## Requirements

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

The server SHALL accept `CONFIG SET <param> <value>` and apply the change immediately for runtime-editable parameters: `cache` (resize the cache byte budget immediately, evicting the LRU tail as needed; `auto` and `N%` values recomputed against current system memory at set time), `password` (takes effect for subsequent `AUTH` and the auth gate; already-authenticated connections remain valid), `cache_file` (empty disables persistence; non-empty enables manual and configured automatic save behavior), `cache_save` (empty disables periodic saves; otherwise a positive Go duration that replaces the live schedule), `cache_save_on_shutdown` (boolean), and `cache_save_rate_limit` (zero/unlimited or a positive human-readable bytes-per-second rate). Startup-only `cache_load`, `cache_restore_limit`, and `cache_restore_reserve` SHALL be visible to `CONFIG GET` but rejected by `CONFIG SET` as read-only/restart-required. The reply SHALL be `OK`. Invalid snapshot values SHALL return an error and preserve the previous effective configuration and scheduler.

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

#### Scenario: Snapshot file enables and disables persistence live

- **WHEN** `CONFIG SET cache_file /var/lib/emb/cache.bin` is issued on a server with a cache
- **THEN** subsequent manual, periodic, and shutdown saves SHALL target that path
- **WHEN** `CONFIG SET cache_file ""` is subsequently issued
- **THEN** no new save SHALL be accepted or scheduled
- **AND** an already-active save MAY finish against the path it captured at acceptance time

#### Scenario: Snapshot interval replaces scheduler

- **WHEN** `CONFIG SET cache_save 5m` is issued while `cache_file` is configured
- **THEN** exactly one scheduler SHALL use the five-minute interval
- **WHEN** it is changed to `1h` or empty
- **THEN** the old schedule SHALL stop and exactly the new schedule or no schedule SHALL remain

#### Scenario: Shutdown and save-rate controls apply live

- **WHEN** `CONFIG SET cache_save_on_shutdown false` is issued
- **THEN** the next graceful shutdown SHALL skip its final save
- **WHEN** `CONFIG SET cache_save_rate_limit 100MB/s` is issued
- **THEN** subsequent accepted saves SHALL use that limit without restarting the coordinator

#### Scenario: Startup-only restore settings reject live mutation

- **WHEN** `CONFIG SET cache_load false`, `CONFIG SET cache_restore_limit 1GB`, or `CONFIG SET cache_restore_reserve 20%` is issued
- **THEN** the server SHALL reject the setting as read-only/restart-required
- **AND** `CONFIG GET` SHALL continue to report the boot value

#### Scenario: Invalid value rejected

- **WHEN** `CONFIG SET cache nonsense`, `CONFIG SET cache 150%`, `CONFIG SET cache_save nonsense`, `CONFIG SET cache_save_on_shutdown maybe`, or `CONFIG SET cache_save_rate_limit -1MB/s` is issued
- **THEN** the server SHALL reply with an error
- **AND** the relevant cache budget, persistence setting, and scheduler SHALL remain unchanged

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

### Requirement: Snapshot configuration is available at startup
The YAML configuration and CLI SHALL accept `cache_file`, `cache_load`, `cache_save`, `cache_save_on_shutdown`, `cache_restore_limit`, `cache_restore_reserve`, and `cache_save_rate_limit`. Defaults SHALL be empty file, load enabled, empty periodic interval, shutdown save enabled when a file exists, automatic restore limit, 10-percent host reserve, and unlimited save rate. Sizes and rates SHALL accept documented human-readable values; restore limit additionally accepts `auto` or total-RAM percentages and reserve accepts bytes or total-RAM percentages. A non-empty `cache_save` interval without a non-empty `cache_file` SHALL be rejected because it cannot produce a snapshot; the other dormant defaults SHALL remain valid when persistence is disabled.

#### Scenario: Valid startup configuration
- **WHEN** the server starts with `cache_file: /var/lib/emb/cache.bin`, `cache_load: true`, `cache_save: 10m`, `cache_save_on_shutdown: true`, `cache_restore_limit: auto`, `cache_restore_reserve: 10%`, and `cache_save_rate_limit: 100MB/s`
- **THEN** startup restore SHALL use that path
- **AND** one ten-minute periodic scheduler SHALL become active
- **AND** restore and saving SHALL use the resolved memory and I/O controls

#### Scenario: Invalid startup combination
- **WHEN** `cache_save` is non-empty with an empty `cache_file`, a duration is zero/negative/unparsable, a boolean is invalid, a size/rate is negative/unparsable, a percentage exceeds 100, or the reserve cannot leave any legal memory
- **THEN** configuration parsing SHALL fail with a clear error before the server listens

### Requirement: CONFIG documented in EMB.HELP

`EMB.HELP` SHALL document `CONFIG GET` and `CONFIG SET` with their parameter usage.

#### Scenario: Help lists the commands

- **WHEN** a client sends `EMB.HELP`
- **THEN** the response SHALL include lines for `CONFIG GET` and `CONFIG SET` describing the syntax