## ADDED Requirements

### Requirement: Graceful shutdown on SIGINT and SIGTERM

The server SHALL gracefully shut down when it receives either SIGINT or SIGTERM.

#### Scenario: Shutdown on SIGINT (Ctrl+C)

- **WHEN** the process receives SIGINT
- **THEN** the server SHALL stop accepting new connections
- **THEN** the server SHALL wait for in-flight requests to complete
- **THEN** the server SHALL close all remaining connections
- **THEN** the server SHALL release ONNX Runtime resources
- **THEN** the process SHALL exit with code 0

#### Scenario: Shutdown on SIGTERM (Docker/orchestration)

- **WHEN** the process receives SIGTERM
- **THEN** the server SHALL follow the same shutdown sequence as SIGINT

### Requirement: Shutdown timeout

The server SHALL NOT wait indefinitely for in-flight requests during shutdown.

#### Scenario: In-flight requests complete within timeout

- **WHEN** the server receives a shutdown signal
- **AND** in-flight requests complete within 30 seconds
- **THEN** the server SHALL wait for all requests to complete before closing connections

#### Scenario: In-flight requests exceed timeout

- **WHEN** the server receives a shutdown signal
- **AND** in-flight requests do not complete within 30 seconds
- **THEN** the server SHALL close remaining connections after the timeout

### Requirement: Clients receive error during shutdown

The server SHALL reject new requests during shutdown with a clear error message.

#### Scenario: New request during shutdown

- **WHEN** the server is shutting down
- **AND** a client sends a new EMB command
- **THEN** the server SHALL respond with a RESP error "ERR server shutting down"
