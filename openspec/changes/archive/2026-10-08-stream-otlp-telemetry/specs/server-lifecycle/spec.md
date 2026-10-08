# Spec Delta

## MODIFIED Requirements

### Requirement: Graceful shutdown on SIGINT and SIGTERM

The server SHALL gracefully shut down when it receives either SIGINT or SIGTERM. It SHALL stop accepting new work, drain in-flight inference, perform a bounded final cache snapshot when configured and dirty, flush pending telemetry when configured, close connections, and release ONNX Runtime resources in that order.

#### Scenario: Shutdown on SIGINT (Ctrl+C)

- **WHEN** the process receives SIGINT
- **THEN** the server SHALL stop accepting new connections
- **THEN** the server SHALL wait for in-flight requests to complete
- **THEN** a configured dirty cache SHALL complete or attempt its bounded final snapshot
- **THEN** the server SHALL flush pending telemetry when configured, within the shutdown deadline
- **THEN** the server SHALL close all remaining connections
- **THEN** the server SHALL release ONNX Runtime resources
- **THEN** the process SHALL exit with code 0

#### Scenario: Shutdown on SIGTERM (Docker/orchestration)

- **WHEN** the process receives SIGTERM
- **THEN** the server SHALL follow the same shutdown sequence as SIGINT

#### Scenario: Telemetry flush failure is non-fatal

- **WHEN** the server receives a shutdown signal and the telemetry endpoint is unreachable
- **THEN** the failed flush SHALL NOT prevent shutdown or change the exit status
