# server-lifecycle Specification

## Purpose
Specifies server lifecycle: graceful shutdown on SIGINT/SIGTERM, a shutdown timeout that lets in-flight requests drain, and errors for requests arriving during shutdown.

## Requirements
### Requirement: Graceful shutdown on SIGINT and SIGTERM

The server SHALL gracefully shut down when it receives either SIGINT or SIGTERM. It SHALL stop accepting new work, drain in-flight inference, perform a bounded final cache snapshot when configured and dirty, close connections, and release ONNX Runtime resources in that order.

#### Scenario: Shutdown on SIGINT (Ctrl+C)

- **WHEN** the process receives SIGINT
- **THEN** the server SHALL stop accepting new connections
- **THEN** the server SHALL wait for in-flight requests to complete
- **THEN** a configured dirty cache SHALL complete or attempt its bounded final snapshot
- **THEN** the server SHALL close all remaining connections
- **THEN** the server SHALL release ONNX Runtime resources
- **THEN** the process SHALL exit with code 0

#### Scenario: Shutdown on SIGTERM (Docker/orchestration)

- **WHEN** the process receives SIGTERM
- **THEN** the server SHALL follow the same shutdown sequence as SIGINT

### Requirement: Shutdown timeout

The server SHALL NOT wait indefinitely for in-flight requests or cache snapshot completion during shutdown. Both operations SHALL share the existing shutdown context deadline; snapshot failure or timeout SHALL be non-fatal to termination.

#### Scenario: In-flight requests and save complete within timeout

- **WHEN** the server receives a shutdown signal
- **AND** in-flight requests and any required final snapshot complete within 30 seconds
- **THEN** the server SHALL wait for both before closing connections and model resources

#### Scenario: In-flight requests or save exceed timeout

- **WHEN** the server receives a shutdown signal
- **AND** in-flight requests or a required snapshot do not complete within 30 seconds
- **THEN** the server SHALL close remaining connections and resources after the timeout
- **AND** the last previously completed atomic snapshot SHALL remain usable

### Requirement: Startup restore precedes readiness
When persistence is configured and `cache_load=true`, host-memory-bounded streaming snapshot parsing and staging SHALL occur before the server reports ready. When loading is disabled, or snapshot data is missing or recoverably invalid, the server SHALL proceed with an empty or safely partial cache rather than fail startup.

#### Scenario: Readiness follows restore attempt
- **WHEN** the server starts with `cache_file` configured
- **THEN** `EMB.READY` SHALL NOT report ready until the initial restore attempt finishes
- **AND** a missing, corrupt, unsupported, or incompatible snapshot SHALL not prevent readiness

#### Scenario: Restore respects current host memory before readiness
- **WHEN** the snapshot is larger than the effective cache/restore/headroom limit
- **THEN** readiness SHALL wait only for bounded streaming validation and permitted admission
- **AND** admitted entries SHALL not exceed the sampled safe memory ceiling

### Requirement: Clients receive error during shutdown

The server SHALL reject new requests during shutdown with a clear error message.

#### Scenario: New request during shutdown

- **WHEN** the server is shutting down
- **AND** a client sends a new EMB command
- **THEN** the server SHALL respond with a RESP error "ERR server shutting down"
