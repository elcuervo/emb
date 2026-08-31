# server-connection-lifecycle Specification

## Purpose
Defines the server's connection and request-bounding lifecycle: idle-connection reaping (a 15-minute TTL by default, explicit `0` to disable) and opt-in caps on accepted connections and concurrent in-flight requests.

## ADDED Requirements

### Requirement: Idle connections are reaped by default with a TTL

The server SHALL close connections that have not sent a command for the configured `idle_timeout` duration; when `idle_timeout` is unset the default TTL SHALL apply (15 minutes); an explicit `0` SHALL disable reaping entirely.

#### Scenario: Idle connection closed after configured timeout

- **GIVEN** a server started with `-idle-timeout 1s`
- **WHEN** a client connects and sends no command
- **THEN** the server SHALL close that connection after ~1 second of inactivity
- **THEN** the connection count reported by `EMB.STATS` SHALL reflect the close

#### Scenario: Active connection survives the timeout

- **GIVEN** a server started with `-idle-timeout 1s`
- **WHEN** a client sends commands at a pace that never leaves it idle for 1 second
- **THEN** the server SHALL NOT close the connection

#### Scenario: Unset idle_timeout applies the default TTL

- **GIVEN** a server started without `-idle-timeout`
- **THEN** `EMB.STATS` SHALL report `idle_timeout_ms` equal to the default TTL in milliseconds (900000)
- **WHEN** a client connects and stays idle beyond that TTL
- **THEN** the server SHALL close the connection

#### Scenario: Explicit zero disables reaping

- **GIVEN** a server started with `-idle-timeout 0`
- **WHEN** a client connects and stays idle
- **THEN** the server SHALL NOT close the connection
- **THEN** `EMB.STATS` SHALL report `idle_timeout_ms: 0`

### Requirement: Accepted connections are capped when configured

The server SHALL refuse connections beyond the configured `max_connections`, counting only connections that were accepted, and SHALL leave the count uncapped when `max_connections` is `0` (default).

#### Scenario: Refuse connection at the cap

- **GIVEN** a server started with `-max-connections 1`
- **WHEN** a second client connects while the first is still connected
- **THEN** the second connection SHALL be closed without being counted
- **THEN** the first connection SHALL continue to operate normally

#### Scenario: Cap counts opens and closes

- **GIVEN** a server started with `-max-connections 2`
- **WHEN** one of two connected clients disconnects
- **THEN** a later connection SHALL be accepted, since the count is below the cap

#### Scenario: Unlimited by default

- **GIVEN** a server started without `-max-connections`
- **THEN** the server SHALL accept connections without an upper bound

### Requirement: Concurrent requests are capped when configured

The server SHALL reject `EMB` and `EMB.MULTI` requests that would exceed the configured `max_concurrent_requests` with a clear RESP error, and SHALL allow unlimited concurrency when `max_concurrent_requests` is `0` (default).

#### Scenario: Busy error at the request cap

- **GIVEN** a server started with `-max-concurrent-requests 1`
- **WHEN** a second `EMB`/`EMB.MULTI` request arrives while one is still being processed
- **THEN** the second request SHALL receive a RESP error beginning with `ERR busy`
- **THEN** the first request SHALL complete normally

#### Scenario: Requests below the cap proceed

- **GIVEN** a server started with `-max-concurrent-requests 4`
- **WHEN** fewer than 4 `EMB` requests are in flight
- **THEN** all of them SHALL be processed normally

#### Scenario: Unlimited by default

- **GIVEN** a server started without `-max-concurrent-requests`
- **THEN** the server SHALL accept concurrent requests without an upper bound

### Requirement: Non-command and admin commands are exempt from the request cap


The request cap SHALL apply only to `EMB` and `EMB.MULTI` work; control commands (`PING`, `AUTH`, `EMB.READY`, `EMB.STATS`, `EMB.MODELS`, `EMB.INFO`, `EMB.HELP`) SHALL always be answered.

#### Scenario: Control commands during saturation

- **GIVEN** a server where `max_concurrent_requests` is reached
- **WHEN** a client sends `PING` or `EMB.STATS`
- **THEN** the server SHALL respond normally instead of a busy error