## Why

The emb server speaks the Redis wire protocol but has no authentication. Any client that can reach the port can embed against any model. Adding `AUTH` matching Redis `requirepass` semantics lets operators control access with minimal friction — existing Redis clients already know the protocol.

## What Changes

- Add an optional `password` field to the YAML config
- Add a `-password` CLI flag
- Add an `AUTH` command handler to the server
- When `password` is set, all commands except `PING` and `AUTH` require prior `AUTH <password>`
- When `password` is not set, behavior is identical to today (no auth, no changes)

## Capabilities

### New Capabilities
- `auth-command`: Redis-compatible AUTH command for connection-level password authentication

### Modified Capabilities
(none)

## Impact

- `internal/config/config.go` — new `Password` field on `Config`, new `-password` flag in `ParseFlags`
- `internal/server/server.go` — accept `password` in `New`, register AUTH handler, inline auth guard in the dispatch closure with `isExempt` and `isAuthenticated` helpers
- `internal/server/server_test.go` — new tests for all auth scenarios
- `cmd/emb/main.go` — pass `fc.Password` to `server.New`
- `config.yaml` — optional `password` field (commented out by default)
