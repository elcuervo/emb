# Spec Delta

## ADDED Requirements

### Requirement: Server section reports the shutdown countdown
While a shutdown grace period is running, the `# Server` section SHALL include `shutdown_in_milliseconds`: the grace deadline minus the current time, in integer milliseconds, as Redis computes it. The field SHALL be absent at all other times, including after an abort.

#### Scenario: During the grace
- **GIVEN** a server with `shutdown_timeout` 30 that received SIGTERM about 10 seconds ago, with a client still connected
- **WHEN** that client sends `INFO server`
- **THEN** the reply SHALL contain `shutdown_in_milliseconds:` with a value no greater than 20000

#### Scenario: Normal operation and after an abort
- **WHEN** a client sends `INFO server` to a server that is not in a grace, including one whose shutdown was aborted
- **THEN** the reply SHALL NOT contain `shutdown_in_milliseconds`
