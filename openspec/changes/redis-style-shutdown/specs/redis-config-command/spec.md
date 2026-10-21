# Spec Delta

## ADDED Requirements

### Requirement: Shutdown grace parameter
The YAML configuration and CLI SHALL accept `shutdown_timeout` (`-shutdown-timeout`), a non-negative integer number of seconds with default `0`. `CONFIG GET` SHALL report it, and `CONFIG SET` SHALL change it with the same validation. A change SHALL apply to the next shutdown, not to a grace already running.

#### Scenario: Startup configuration
- **WHEN** the server starts with `shutdown_timeout: 15`
- **THEN** `CONFIG GET shutdown_timeout` SHALL report `15`

#### Scenario: Runtime change
- **WHEN** `CONFIG SET shutdown_timeout 30` is issued
- **THEN** `CONFIG GET shutdown_timeout` SHALL report `30`
- **AND** the next shutdown SHALL use a 30-second grace

#### Scenario: Invalid values
- **WHEN** `CONFIG SET shutdown_timeout -1` or `CONFIG SET shutdown_timeout 1.5` is issued
- **THEN** the server SHALL reply with an error and keep the previous value

### Requirement: Signal shutdown parameters
The YAML configuration and CLI SHALL accept `shutdown_on_sigterm` (`-shutdown-on-sigterm`) and `shutdown_on_sigint` (`-shutdown-on-sigint`). Each is a space-separated combination of `default`, `save`, `nosave`, `now`, and `force`, default `default`, and `save` with `nosave` is invalid. `CONFIG GET` SHALL report them, and `CONFIG SET` SHALL change them with the same validation.

#### Scenario: Startup configuration
- **WHEN** the server starts with `shutdown_on_sigint: nosave now`
- **THEN** `CONFIG GET shutdown_on_*` SHALL report `shutdown_on_sigint` `nosave now` and `shutdown_on_sigterm` `default`

#### Scenario: Invalid values
- **WHEN** `CONFIG SET shutdown_on_sigterm "save nosave"` or `CONFIG SET shutdown_on_sigterm later` is issued
- **THEN** the server SHALL reply with an error and keep the previous value

#### Scenario: Invalid startup value
- **WHEN** the server starts with `shutdown_on_sigterm: save nosave`
- **THEN** startup SHALL fail with an error naming `shutdown_on_sigterm`

### Requirement: Redis hyphenated parameter names
`CONFIG GET` patterns and `CONFIG SET` names SHALL treat `-` and `_` as equivalent, so Redis spellings such as `shutdown-timeout` address the same parameter as `shutdown_timeout`. Replies SHALL name parameters in their canonical underscore spelling.

#### Scenario: Redis tooling sets the grace
- **WHEN** `CONFIG SET shutdown-timeout 10` is issued
- **THEN** the server SHALL reply `+OK`
- **AND** `CONFIG GET shutdown-timeout` SHALL reply with the pair `shutdown_timeout` `10`

#### Scenario: Existing parameter by Redis spelling
- **WHEN** `CONFIG GET cache-save-on-shutdown` is issued
- **THEN** the reply SHALL contain `cache_save_on_shutdown` and its current value
