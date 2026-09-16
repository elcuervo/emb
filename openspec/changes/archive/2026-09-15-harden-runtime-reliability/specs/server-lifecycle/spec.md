## ADDED Requirements

### Requirement: Atomic inference admission and drain transition
Admission for EMB, EMB.MULTI, EMB.IMG, EMB.IMGMULTI, EMB.EVAL, and EMB.EVSHA SHALL atomically check the concurrency cap and shutdown state and register accepted work. Once shutdown has stopped admission, no new inference work SHALL be registered. Saturation SHALL NOT consume control-command capacity.

#### Scenario: Simultaneous requests compete for the final slot
- **WHEN** multiple inference requests arrive concurrently with one slot remaining
- **THEN** at most one SHALL be admitted and the others SHALL receive ERR busy responses

#### Scenario: Request races shutdown
- **WHEN** admission races the shutdown transition
- **THEN** the request SHALL either be registered before draining and awaited or rejected with ERR server shutting down
- **AND** it SHALL NOT escape shutdown accounting

### Requirement: Transport and process preserve graceful completion
Stopping new connections SHALL preserve accepted client connections until their accepted work and replies complete or the shutdown deadline expires. The entrypoint SHALL join shutdown before normal return and destroy the ONNX environment only after native resource users have stopped.

#### Scenario: Signal arrives during inference
- **WHEN** SIGTERM arrives while an accepted inference is running and it finishes within the shutdown deadline
- **THEN** the client SHALL receive its reply before its connection closes
- **AND** model resource release and environment destruction SHALL follow completed inference and any bounded final snapshot

## MODIFIED Requirements

### Requirement: Shutdown timeout

The server SHALL NOT wait indefinitely for in-flight requests or cache snapshot completion during shutdown. Both operations SHALL share the existing shutdown context deadline; snapshot failure or timeout SHALL be non-fatal to termination. Native resources SHALL NOT be destroyed while a native call is still using them; if such a call outlives the deadline, the process SHALL terminate and rely on operating-system reclamation instead of concurrent native destruction.

#### Scenario: In-flight requests and save complete within timeout

- **WHEN** the server receives a shutdown signal
- **AND** in-flight requests and any required final snapshot complete within 30 seconds
- **THEN** the server SHALL wait for both before closing connections and model resources

#### Scenario: In-flight requests or save exceed timeout

- **WHEN** the server receives a shutdown signal
- **AND** in-flight requests or a required snapshot do not complete within 30 seconds
- **THEN** the server SHALL close remaining connections and terminate without unbounded cleanup waits
- **AND** resources still in use by native inference SHALL be reclaimed by process termination, not concurrent native destruction
- **AND** the last previously completed atomic snapshot SHALL remain usable
