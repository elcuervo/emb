## Context

The current shutdown path (lines 43-52 in `cmd/emb/main.go`):
1. Catches only `os.Interrupt` (SIGINT)
2. Closes redcon server immediately (active connections cut)
3. Closes model sessions
4. Calls `os.Exit(0)` — skips `onnx.DestroyEnvironment()` and other deferred cleanup

`redcon` has no built-in graceful shutdown. Its `Close()` method immediately closes the listener and all connections. In-flight ONNX inferences (which can take 10-50ms each) get aborted, and clients see connection resets.

## Goals / Non-Goals

**Goals:**
- Catch both SIGINT and SIGTERM
- Stop accepting new connections on signal
- Wait for in-flight requests (with timeout) before closing
- Return clean RESP errors to clients still connected during shutdown
- Let `main()` return normally so deferred cleanup runs
- 30-second default shutdown timeout

**Non-Goals:**
- Protocol-level graceful shutdown (RESP has no draining protocol)
- Persisting in-flight state across restarts
- Configurable timeout (can be added later)
- Hot reload of config (separate concern)

## Decisions

### Signal handling: SIGINT + SIGTERM, clean return

Replace the goroutine with `os.Exit(0)` with a shutdown function that signals `main()` to return normally:

```
main:
  srv := New(listen, reg)
  sigCh := make(chan os.Signal, 1)
  signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

  go func() {
    <-sigCh
    log.Print("shutting down...")
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    srv.Shutdown(ctx)  // stops accepting, drains, then closes
    reg.Close()
    done <- struct{}{}
  }()

  srv.ListenAndServe()  // blocks
  <-done  // wait for drain
  // main returns → defer onnx.DestroyEnvironment() runs
```

### Connection draining: accept then drain

`redcon` doesn't expose connection tracking. Approach:

1. Create a `net.Listener` manually (`net.Listen("tcp", addr)`)
2. Pass to `srv.Serve(ln)` instead of `srv.ListenAndServe()`
3. Add an `active sync.WaitGroup` to `Server`, incremented in handler entry, decremented on response
4. On `Shutdown(ctx)`:
   - Close `ln` (stops accepting)
   - Wait for `active` WaitGroup with ctx deadline
   - Close redcon server (remaining connections go down)

```
Shutdown:
  ln.Close()           → Serve() returns error
  active.Wait() ...or ctx.Done()
  srv.Close()          → kills remaining conns
```

The listener is held on the `Server` struct so `Shutdown` can close it.

### RESP error on draining connections

During shutdown, pending connections get an error message before close. Since redcon `Close()` kills them immediately, the error can't be sent via RESP. Instead, set a `shuttingDown atomic.Bool` on the Server — handlers check it before processing:

```go
func (s *Server) handleEMB(conn redcon.Conn, cmd redcon.Command) {
    if s.shuttingDown.Load() {
        conn.WriteError("ERR server shutting down")
        return
    }
    s.active.Add(1)
    defer s.active.Done()
    // ...
}
```

This only catches requests that arrive at the handler. Requests already in-flight are drained via the WaitGroup. Any connection that hasn't started a new command by the time the timeout fires gets closed by redcon.

### Shutdown timeout: 30 seconds

30 seconds covers:
- Any single ONNX inference (typically 1-50ms with batching enabled)
- Tokenizer encoding of long texts (typically <1ms)
- Network buffering and RESP response transmission (<1s)

| Timeout | Risk |
|---------|------|
| 5s | May cut off batched requests with long texts |
| 30s | Generous for any single request |
| 60s+ | Unnecessary delay in container orchestration |

## Risks / Trade-offs

- [redcon has no built-in draining support] → Manual `sync.WaitGroup` + custom listener approach. Works for current architecture. If redcon adds native support later, can simplify.
- [30-second timeout is arbitrary] → Models with very long max_length (e.g., 8192 tokens) could exceed 30s for tokenization. Acceptable — if a request takes longer than 30s in practice, the config's `max_length` should be adjusted.
- [Handlers must increment active counter] → Easy to forget in a new handler. Mitigation: use a wrapper or middleware pattern if more handlers are added.
