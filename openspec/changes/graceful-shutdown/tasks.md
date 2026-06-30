## 1. Add Shutdown support to server package

- [x] 1.1 Add `active sync.WaitGroup` and `shuttingDown atomic.Bool` fields to `Server` struct
- [x] 1.2 Add `ln net.Listener` field to `Server` struct for manual listener management
- [x] 1.3 Update `ListenAndServe()`: create `net.Listener` via `net.Listen("tcp", addr)`, pass to `srv.Serve(ln)`
- [x] 1.4 Add `increment()` and `decrement()` helpers (or use WaitGroup directly) for active request tracking
- [x] 1.5 Add `Shutdown(ctx context.Context) error` method: close listener, wait for active requests (with ctx timeout), close redcon server
- [x] 1.6 Update all handlers (`handleEMB`, `handleEMBMULTI`) to check `shuttingDown` and increment/decrement active counter

## 2. Update main.go signal handling

- [x] 2.1 Import `syscall` for SIGTERM, `context` for shutdown timeout
- [x] 2.2 Replace current signal goroutine: catch `syscall.SIGINT` + `syscall.SIGTERM`
- [x] 2.3 On signal: call `srv.Shutdown(ctx)` with 30s timeout, then `reg.Close()`
- [x] 2.4 Remove `os.Exit(0)` — let main return normally (deferred `onnx.DestroyEnvironment` runs)

## 3. Verify

- [x] 3.1 `go build ./...` — compiles
- [x] 3.2 `go vet ./...` — passes
- [x] 3.3 `go test ./...` — all pass
- [x] 3.4 Manual: SIGTERM → "shutting down..." → exit 0, "server stopped" logged
- [x] 3.5 Docker stop: `docker stop` sends SIGTERM (verified via terminal simulation)
- [ ] 3.6 Manual: in-flight EMB request completes during shutdown (verify via log timing)
