# client-global-configuration Specification

## Purpose
Specifies a global configuration layer for the `emb` Ruby gem so every client (default,
`Emb.setup`, and `Emb.new`) picks up evidence-based out-of-the-box defaults — and
operators can override them once via `Emb.configure` — instead of repeating options at
every call site. `EMB_URL` remains the only environment variable.

## ADDED Requirements

### Requirement: Opt-in batch retries via reconnect_attempts

The gem SHALL keep `reconnect_attempts: 0` as the default (a failing batch fails closed
after a single attempt), while allowing operators to opt into bounded retries by setting
`reconnect_attempts` to a value greater than zero: a transiently failing `EMB.MULTI` in a
batch is then re-sent up to that many additional times before the batch fails closed with
`Emb::ServerError`. Operation errors SHALL never be retried, regardless of the setting.
The retry budget SHALL be capped by `reconnect_attempts` and every exhausted batch SHALL
terminate in a visible `Emb::ServerError`, never an endless silent re-send.

#### Scenario: Default fails closed after one attempt

- **WHEN** an `EMB.MULTI` fails transiently under the default configuration (`reconnect_attempts: 0`)
- **THEN** exactly one command SHALL have been sent for the batch
- **AND** the batch SHALL fail closed by raising `Emb::ServerError`

#### Scenario: Setting reconnect_attempts engages retries

- **WHEN** `Emb.configure { |c| c.reconnect_attempts = 2 }` is set
- **AND** an `EMB.MULTI` fails transiently
- **THEN** up to 3 commands SHALL be sent for the batch (the initial attempt plus 2 retries)
- **AND** if all three fail, the batch SHALL fail closed by raising `Emb::ServerError`

#### Scenario: Operation errors never retry

- **WHEN** `Emb.configure { |c| c.reconnect_attempts = 2 }` is set
- **AND** the server returns an error reply for the batch
- **THEN** exactly one command SHALL have been sent
- **AND** the batch SHALL fail closed by raising `Emb::ServerError`

#### Scenario: Defaults documented

- **WHEN** a contributor reads the gem README
- **THEN** they SHALL find the out-of-the-box config (batching on, pool 5, driver, 10s timeouts, `reconnect_attempts: 0`), the failure taxonomy (transient errors retry when configured, operation errors never), and how to override globally or per call