## Context

The server currently tracks no lifecycle state beyond a `started` timestamp and a `shuttingDown` atomic bool. The registry holds a set of `ModelEntry` values, each with a `loaded` atomic that flips true once `ensurePool()` succeeds. Models with `preload: true` are loaded during registration; models without it load lazily on first access.

For EMB.READY we need to expose: server state (loading/ready/draining), model loading progress, and uptime — without introducing complex state machines.

## Goals / Non-Goals

**Goals:**
- Three-state readiness: loading → ready → draining (done)
- EMB.READY returns a boolean-ready signal for LB health checks
- PING stays dumb (layer 4 health checks unaffected)
- Draining set on SIGTERM before shutdown
- Ruby gem exposes `Emb.ready?` (boolean) and `Emb.ready` (reason string)

**Non-Goals:**
- Structured data in EMB.READY response (use EMB.STATS for detail)
- Changing shutdown semantics — we set the drain flag, then proceed with existing graceful shutdown

## Decisions

1. **Three `iota` states in `server.go`.** Not a string enum. An int8 `serverState` type with `stateLoading`, `stateReady`, `stateDraining`. Set atomically. The handler reads it and builds the response. No mutex contention on the hot path.

2. **`Registry.ModelsLoaded()` returns `(loaded, total)`.** Registry already has `List()` which returns all entries. We count how many have `loaded == true` vs total configured. Total is known at construction time — store it as `modelCount int`.

3. **Response is a status string, not a key-value array.** The LB needs a boolean signal, not structured data. `EMB.READY` returns `+OK` when ready or `-ERR <reason>` when not:

   ```
   Client: EMB.READY
   Server: +OK                  (ready to serve)
   Server: -ERR loading         (models still loading)
   Server: -ERR draining        (SIGTERM received)
   Server: -ERR no models       (no models configured)
   ```

   The error message after `-ERR` is a human-readable reason. Load balancers check `+OK` / `-ERR` prefix — no parsing needed.

4. **Draining flag set before `ln.Close()` in shutdown path.** In `cmd/emb/main.go`, on SIGTERM we first set draining, then initiate shutdown. The LB polling interval (usually 5-15s) ensures it sees draining before the port closes.

5. **No config changes needed.** EMB.READY always works. No password protection (it's an info command, not a sensitive operation). If auth is configured, EMB.READY is exempt (like PING) so health checks work without credentials.

6. **Ruby gem has `ready?` and `ready`.** `Client#ready?` returns `true` if `+OK`, `false` otherwise. `Client#ready` returns the reason string (`"ready"`, `"loading"`, `"draining"`, `"no models"`). Both delegate from the `Emb` module.

## Risks / Trade-offs

- **Draining race** — if the LB polls EMB.READY every 10s and the shutdown timeout is 30s, we need to set draining fast. The current shutdown path closes the listener immediately, then waits for in-flight requests. We flip draining before closing the listener, giving the LB at least one poll cycle to react.
- **Loading vs ready transition** — the server starts in `loading` state. After all preloaded models finish `ensurePool()`, it transitions to `ready`. Non-preloaded models don't delay readiness. This means a server with zero preloaded models goes instantly to `ready`, which is correct — it'll load models on first use.
