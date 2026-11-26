# Spec Delta

## ADDED Requirements

### Requirement: Shutdown requests
SIGTERM, SIGINT, and the `SHUTDOWN` command SHALL each request the same shutdown sequence. A signal SHALL use the flags configured in `shutdown_on_sigterm` or `shutdown_on_sigint`. The command SHALL use its own arguments. Flags SHALL be `SAVE`, `NOSAVE`, `NOW`, and `FORCE`, with the same meaning for signals and the command.

#### Scenario: Signal and command run the same sequence
- **GIVEN** two servers with identical configuration
- **WHEN** one receives SIGTERM with `shutdown_on_sigterm: default` and the other receives `SHUTDOWN` from an authorized client
- **THEN** both SHALL run the same grace, connection-close, drain, snapshot, and exit steps

#### Scenario: Save flags
- **GIVEN** a server with `cache_file` set and a dirty cache
- **WHEN** it shuts down with `NOSAVE`
- **THEN** it SHALL NOT write a final snapshot, even when `cache_save_on_shutdown` is true
- **WHEN** it shuts down with `SAVE`
- **THEN** it SHALL write a final snapshot, even when `cache_save_on_shutdown` is false

#### Scenario: SAVE without a snapshot file
- **GIVEN** a server without `cache_file`
- **WHEN** it shuts down with `SAVE`
- **THEN** it SHALL log that no snapshot file is configured and continue the shutdown

### Requirement: Shutdown grace period
When a shutdown is requested without `NOW`, `shutdown_timeout` is greater than zero, and clients other than blocked `SHUTDOWN` callers are connected, the server SHALL enter a grace period before stopping clients. During the grace it SHALL accept connections, execute every command normally, and answer `EMB.READY` with `draining`. The grace SHALL end at the deadline, or once no such clients remain, checked at least every 100ms.

#### Scenario: Commands are served during the grace
- **GIVEN** a server with `shutdown_timeout` 10 and a connected client
- **WHEN** the server receives SIGTERM and the client then sends `EMB`
- **THEN** the client SHALL receive embeddings
- **AND** `EMB.READY` SHALL reply `draining`

#### Scenario: Grace ends once clients leave
- **GIVEN** a server with `shutdown_timeout` 30 and two connected clients
- **WHEN** the server receives SIGTERM and both clients disconnect 2 seconds later
- **THEN** the grace SHALL end within 100ms of the last disconnect

#### Scenario: Grace is bounded
- **GIVEN** a server with `shutdown_timeout` 2 and a client that stays connected
- **WHEN** the server receives SIGTERM
- **THEN** the grace SHALL end 2 seconds later and the shutdown SHALL continue

#### Scenario: No grace without clients, with NOW, or with a zero timeout
- **WHEN** a shutdown is requested with no other clients connected, with `NOW`, or with `shutdown_timeout` 0
- **THEN** the server SHALL skip the grace and stop clients immediately

#### Scenario: Grace does not consume the drain deadline
- **GIVEN** a server with `shutdown_timeout` 10
- **WHEN** the grace runs for its full 10 seconds
- **THEN** in-flight work and the final snapshot SHALL still get the full shutdown deadline after the grace ends

### Requirement: Clients are stopped without error replies
When the grace ends, or immediately when there is none, the server SHALL:
- close its listening socket, so new connections are refused;
- stop executing new commands;
- close every connection that has no command in progress;
- close each remaining connection right after its running command's reply is written.

A command not yet executing when clients are stopped SHALL receive no reply. No shutdown-specific RESP error SHALL be sent.

#### Scenario: Idle pooled connection
- **WHEN** clients are stopped while a client holds an idle connection
- **THEN** the server SHALL close that connection
- **AND** the client's next command SHALL fail with a connection error, not a RESP error

#### Scenario: Command in progress
- **WHEN** clients are stopped while a client's `EMB` command is executing
- **THEN** the client SHALL receive that command's reply
- **AND** the server SHALL then close the connection

#### Scenario: Pipelined command behind a running one
- **WHEN** clients are stopped while a connection has one command executing and a second pipelined behind it
- **THEN** the first command SHALL receive its reply
- **AND** the connection SHALL close without a reply to the second

#### Scenario: New connection after clients are stopped
- **WHEN** a client connects after clients are stopped
- **THEN** the connection SHALL be refused

### Requirement: SHUTDOWN command
The server SHALL accept `SHUTDOWN [NOSAVE|SAVE] [NOW] [FORCE] [ABORT]`, with flags matched case-insensitively. It SHALL reply `-ERR syntax error` to an unknown flag, to `SAVE` with `NOSAVE`, and to `ABORT` combined with any other flag, and start nothing. Otherwise the caller SHALL block. It SHALL receive no reply on success, because its connection closes. If a shutdown is already in progress, a new `SHUTDOWN` SHALL block until that one ends, and its flags SHALL be ignored.

#### Scenario: Shutdown through the command
- **GIVEN** a server with `shutdown_timeout` 10 and another connected client
- **WHEN** an authorized client sends `SHUTDOWN`
- **THEN** that client SHALL receive no reply while the grace runs
- **AND** its connection SHALL close when clients are stopped, and the process SHALL exit 0

#### Scenario: Immediate termination idiom
- **WHEN** an authorized client sends `SHUTDOWN NOW NOSAVE FORCE`
- **THEN** the server SHALL skip the grace and the final snapshot and exit 0

#### Scenario: Invalid arguments
- **WHEN** a client sends `SHUTDOWN SAVE NOSAVE`, `SHUTDOWN ABORT NOW`, or `SHUTDOWN LATER`
- **THEN** the server SHALL reply `-ERR syntax error` and keep running normally

#### Scenario: Second caller during the grace
- **WHEN** a second client sends `SHUTDOWN NOSAVE` while a grace started by SIGTERM is running
- **THEN** that client SHALL block
- **AND** the shutdown SHALL keep the flags of the SIGTERM

### Requirement: SHUTDOWN ABORT
`SHUTDOWN ABORT` received during a grace period SHALL cancel the shutdown, whether a signal or the command started it. The server SHALL then return to normal operation: `EMB.READY` replies `OK` and `INFO` no longer reports `shutdown_in_milliseconds`. It SHALL reply `-ERR Errors trying to SHUTDOWN. Check logs.` to every blocked `SHUTDOWN` caller, and `+OK` to the aborting client. With no shutdown in progress, it SHALL reply `-ERR No shutdown in progress.`.

#### Scenario: Abort during the grace
- **GIVEN** a server with `shutdown_timeout` 30, a client blocked in `SHUTDOWN`, and a second client
- **WHEN** the second client sends `SHUTDOWN ABORT`
- **THEN** the second client SHALL receive `+OK`
- **AND** the blocked client SHALL receive `-ERR Errors trying to SHUTDOWN. Check logs.`
- **AND** `EMB.READY` SHALL reply `OK` and the process SHALL keep running

#### Scenario: Nothing to abort
- **WHEN** a client sends `SHUTDOWN ABORT` to a server that is not shutting down
- **THEN** the server SHALL reply `-ERR No shutdown in progress.`

#### Scenario: A later shutdown after an abort
- **WHEN** a shutdown is aborted and the server later receives SIGTERM
- **THEN** the server SHALL run the full shutdown sequence again

### Requirement: Second SIGINT exits immediately
A SIGINT received while a shutdown is already pending or in its grace SHALL make the process log `You insist... exiting now.` and exit immediately with status 1. It SHALL NOT wait for the grace, in-flight work, or a snapshot. A second SIGTERM SHALL NOT change the running shutdown.

#### Scenario: Ctrl+C twice
- **GIVEN** a server with `shutdown_timeout` 30 and a connected client
- **WHEN** the process receives SIGINT and then a second SIGINT
- **THEN** the process SHALL exit with status 1 without writing a snapshot

#### Scenario: Repeated SIGTERM
- **GIVEN** a server in a grace started by SIGTERM
- **WHEN** it receives another SIGTERM
- **THEN** the grace SHALL continue unchanged

### Requirement: Shutdown during startup
A SIGTERM or SIGINT received before the server starts listening, while models load or the snapshot restores, SHALL stop startup at the next step boundary and exit with status 0 without writing a snapshot.

#### Scenario: SIGTERM while loading
- **WHEN** the process receives SIGTERM while a model is preloading
- **THEN** it SHALL exit 0 without opening the listener and without writing a snapshot

### Requirement: SHUTDOWN access control
With a password configured, `SHUTDOWN` SHALL require authentication like other commands. Without a password, the server SHALL accept `SHUTDOWN` only from loopback client addresses. Any other client SHALL receive an error starting with `-DENIED`, and the server SHALL keep running.

#### Scenario: Remote client, no password
- **GIVEN** a server with no password listening on `:6399`
- **WHEN** a non-loopback client sends `SHUTDOWN`
- **THEN** the client SHALL receive a `-DENIED` error and the server SHALL keep running

#### Scenario: Local operator, no password
- **GIVEN** a server with no password
- **WHEN** a client on `127.0.0.1` sends `SHUTDOWN NOW`
- **THEN** the server SHALL shut down

## MODIFIED Requirements

### Requirement: Graceful shutdown on SIGINT and SIGTERM

The server SHALL gracefully shut down when it receives either SIGINT or SIGTERM. It SHALL run the grace period when one applies, then stop clients, drain in-flight inference, perform a bounded final cache snapshot as the shutdown flags and `cache_save_on_shutdown` direct, flush pending telemetry when configured, and release ONNX Runtime resources, in that order.

#### Scenario: Shutdown on SIGINT (Ctrl+C)

- **WHEN** the process receives SIGINT
- **THEN** the server SHALL run the grace period when one applies
- **THEN** the server SHALL close its listening socket and stop executing new commands
- **THEN** the server SHALL close connections without a command in progress, and close the others after their reply
- **THEN** the server SHALL wait for in-flight requests to complete
- **THEN** a configured dirty cache SHALL complete or attempt its bounded final snapshot, unless `NOSAVE` applies
- **THEN** the server SHALL flush pending telemetry when configured, within the shutdown deadline
- **THEN** the server SHALL release ONNX Runtime resources
- **THEN** the process SHALL exit with code 0

#### Scenario: Shutdown on SIGTERM (Docker/orchestration)

- **WHEN** the process receives SIGTERM
- **THEN** the server SHALL follow the same shutdown sequence as SIGINT, using `shutdown_on_sigterm`

#### Scenario: Telemetry flush failure is non-fatal

- **WHEN** the server receives a shutdown signal and the telemetry endpoint is unreachable
- **THEN** the failed flush SHALL NOT prevent shutdown or change the exit status

### Requirement: Atomic inference admission and drain transition
Admission for EMB, EMB.MULTI, EMB.IMG, EMB.IMGMULTI, EMB.EVAL, and EMB.EVSHA SHALL atomically check the concurrency cap and shutdown state, and register accepted work. Once clients are stopped, no new inference work SHALL be registered. Saturation SHALL NOT consume control-command capacity.

#### Scenario: Simultaneous requests compete for the final slot
- **WHEN** multiple inference requests arrive concurrently with one slot remaining
- **THEN** at most one SHALL be admitted and the others SHALL receive ERR busy responses

#### Scenario: Request races shutdown
- **WHEN** admission races the transition to stopped clients
- **THEN** the request SHALL either be registered before the transition and awaited with its reply, or not executed, with its connection closed without a reply
- **AND** it SHALL NOT escape shutdown accounting

## REMOVED Requirements

### Requirement: Clients receive error during shutdown
**Reason**: Redis never answers commands with a shutdown-specific error. Clients handle a closed connection as a retryable connection error but surface RESP errors, so `ERR server shutting down` turned every restart into caller-visible failures.
**Migration**: Callers that matched `ERR server shutting down` should treat a connection error during shutdown as the signal to reconnect, as with Redis. See "Clients are stopped without error replies".
