## ADDED Requirements

### Requirement: EMB.READY command
The server SHALL respond to the `EMB.READY` command with `+OK` when ready or `-ERR <reason>` when not.

#### Scenario: EMB.READY when ready
- **GIVEN** all preloaded models are fully loaded
- **WHEN** a client sends `EMB.READY`
- **THEN** the server responds with `+OK`

#### Scenario: EMB.READY while loading
- **GIVEN** models are configured with `preload: true`
- **WHEN** not all models are fully loaded yet
- **THEN** the server responds with `-ERR loading`

#### Scenario: EMB.READY during draining
- **GIVEN** a SIGTERM has been received
- **WHEN** a client sends `EMB.READY`
- **THEN** the server responds with `-ERR draining`

#### Scenario: EMB.READY with no models
- **GIVEN** no models are configured
- **WHEN** a client sends `EMB.READY`
- **THEN** the server responds with `-ERR no models`

#### Scenario: EMB.READY with no preloaded models
- **GIVEN** no models have `preload: true`
- **WHEN** a client sends `EMB.READY`
- **THEN** the server responds with `+OK` (instantly — models load on demand)

### Requirement: Ruby gem `ready?` and `ready`
The gem SHALL expose `Emb.ready?` returning a boolean and `Emb.ready` returning the reason string.

#### Scenario: ready? when OK
- **WHEN** `EMB.READY` returns `+OK`
- **THEN** `Emb.ready?` returns `true`

#### Scenario: ready? when error
- **WHEN** `EMB.READY` returns `-ERR loading`
- **THEN** `Emb.ready?` returns `false`

#### Scenario: ready returns reason string
- **WHEN** `EMB.READY` returns `-ERR draining`
- **THEN** `Emb.ready` returns `"draining"`

### Requirement: PING remains unchanged
The `PING` command SHALL continue to respond `+PONG` regardless of server state. Layer 4 health checks must keep working.

### Requirement: Graceful shutdown drain
When a SIGTERM is received, the server SHALL set its state to `draining` before closing the listener. This gives load balancers time to mark the instance unhealthy via `EMB.READY` polling.
