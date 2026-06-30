## Why

The server currently handles SIGINT with `os.Exit(0)` after closing connections, but doesn't catch SIGTERM (the standard stop signal from Docker, orchestration, and systemd). Active ONNX inferences are cut short, deferred cleanup (e.g., `onnx.DestroyEnvironment`) is skipped, and clients receive connection resets instead of clean RESP errors.

## What Changes

- Catch both SIGINT and SIGTERM instead of just SIGINT
- Replace `os.Exit(0)` with clean `main()` return so deferred cleanup runs
- Stop accepting new connections on shutdown signal
- Drain active in-flight requests with a configurable timeout before closing
- Return a Redis-compatible error to clients still connected during shutdown

## Capabilities

### New Capabilities
- `server-lifecycle`: Graceful startup and shutdown lifecycle — signal handling, connection draining, request timeout during shutdown

### Modified Capabilities
- (none — no existing spec-level behavior changes; current behavior is not specified)

## Impact

| File | Change |
|------|--------|
| `cmd/emb/main.go` | Signal handling: SIGINT+SIGTERM, no os.Exit, shutdown timeout, clean return |
| `internal/server/server.go` | Add `Shutdown(timeout)` method: stop accepting, drain, close |
| `README.md` | Note shutdown behavior |
