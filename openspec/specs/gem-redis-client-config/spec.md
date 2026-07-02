## Purpose

Allow users to configure any `RedisClient` option (timeouts, SSL, driver, reconnect
backoff, etc.) through the emb Ruby client without requiring a gem update. Only `pool`
is handled by the gem itself.

## Requirements

### Requirement: redis-client options are forwarded through the gem

Users SHALL be able to pass any `RedisClient` constructor option through `Emb.setup`,
`Emb.new`, or `Emb::Client.new` using `**rest`.

#### Scenario: Forward connect_timeout

- **WHEN** a user calls `Emb.setup(url: "redis://localhost:6379", connect_timeout: 2)`
- **THEN** the underlying `RedisClient` SHALL be created with `connect_timeout: 2`
- **THEN** the connection pool SHALL work normally

#### Scenario: Forward ssl options

- **WHEN** a user calls `Emb.setup(url: "redis://localhost:6379", ssl: true, ssl_params: { verify_mode: 0 })`
- **THEN** the underlying `RedisClient` SHALL be created with SSL enabled
- **THEN** the connection pool SHALL work normally

#### Scenario: Forward driver

- **WHEN** a user calls `Emb.setup(url: "redis://localhost:6379", driver: :hiredis)`
- **THEN** the underlying `RedisClient` SHALL use the hiredis driver

#### Scenario: Forward inherit_socket

- **WHEN** a user calls `Emb.setup(url: "redis://localhost:6379", inherit_socket: true)`
- **THEN** the underlying `RedisClient` SHALL be created with `inherit_socket: true`

#### Scenario: Protocol defaults to RESP2

- **WHEN** a user calls `Emb.setup(url: "redis://localhost:6379")`
- **THEN** the underlying `RedisClient` SHALL default to `protocol: 2`
- **AND** if a user passes `protocol: 3`, it SHALL be forwarded as-is and rejected by the server naturally (redcon only speaks RESP2)

#### Scenario: Pool size remains separate

- **WHEN** a user calls `Emb.setup(url: "redis://localhost:6379", pool: 10, connect_timeout: 2)`
- **THEN** `pool: 10` SHALL control the `ConnectionPool` size
- **AND** `connect_timeout: 2` SHALL pass through to `RedisClient`
