## MODIFIED Requirements

### Requirement: redis-client options are forwarded through the gem

Users SHALL be able to pass any `RedisClient` constructor option through `Emb.setup`, `Emb.new`, or `Emb::Client.new` using `**rest`, with one exception: `protocol` is validated by the gem rather than forwarded blindly. The gem speaks RESP2 and does not decode the RESP3 maps the server emits for `EMB.MODELS`, `EMB.STATS`, `EMB.INFO`, and `CONFIG GET`, so any `protocol` other than `2` SHALL raise `ArgumentError` at configuration time (`Emb.configure`) or client construction time (`Emb.new`/`Emb::Client.new`), before any connection is made.

#### Scenario: Forward connect_timeout

- **WHEN** a user calls `Emb.setup(url: "redis://localhost:6379", connect_timeout: 2)`
- **THEN** the underlying `RedisClient` SHALL be created with `connect_timeout: 2`
- **THEN** the connection pool SHALL work normally

#### Scenario: Forward ssl options

- **WHEN** a user calls `Emb.setup(url: "redis://localhost:6379", ssl: true, ssl_params: { verify_mode: OpenSSL::SSL::VERIFY_PEER })`
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
- **AND** when a user passes `protocol: 3` to `Emb.setup`, `Emb.new`, or `Emb.configure`, the gem SHALL raise `ArgumentError` instead of forwarding it, because the client cannot decode the RESP3 replies that mode produces

#### Scenario: Pool size remains separate

- **WHEN** a user calls `Emb.setup(url: "redis://localhost:6379", pool: 10, connect_timeout: 2)`
- **THEN** `pool: 10` SHALL control the `ConnectionPool` size
- **AND** `connect_timeout: 2` SHALL pass through to `RedisClient`
