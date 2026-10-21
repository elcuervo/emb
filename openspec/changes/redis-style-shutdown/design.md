# Design

## Context

See `proposal.md`, section Why. Requirements are in `specs/server-lifecycle`, `specs/redis-config-command`, and `specs/redis-style-info`.

### Redis 7.4 reference behavior

Read from `src/server.c` (`sigShutdownHandler`, `serverCron`, `prepareForShutdown`, `isReadyToShutdown`, `cancelShutdown`, `abortShutdown`, `finishShutdown`, `closeListeningSockets`, `genRedisInfoString`), `src/db.c` (`shutdownCommand`), and `src/blocked.c` (`replyToClientsBlockedOnShutdown`):

1. **Signals.**
   - SIGTERM or SIGINT sets `shutdown_asap`. The next cron tick (`hz` 10, so every 100ms) calls `prepareForShutdown` with the flags from `shutdown-on-sigterm` / `shutdown-on-sigint`.
   - A SIGINT while `shutdown_asap` is already set logs `You insist... exiting now.`, removes the temporary RDB, and calls `exit(1)`.
   - A second SIGTERM only sets the flag again.
   - During loading, a shutdown is forced to `NOSAVE`.
2. **`SHUTDOWN` parsing.**
   - Flags `nosave`, `save`, `now`, `force`, and `abort` are matched case-insensitively.
   - An unknown flag, `abort` with any other flag, or `save` with `nosave` replies `-ERR syntax error`.
   - `abort` replies `+OK`, or `-ERR No shutdown in progress.`.
   - Otherwise the caller is blocked (`blockClientShutdown`) and `prepareForShutdown` runs.
3. **`prepareForShutdown`.**
   - If a shutdown is already initiated it returns immediately, so an extra `SHUTDOWN` caller just stays blocked and its flags are ignored.
   - Without `NOW`, with `shutdown-timeout` non-zero, and with replicas lagging, it sets `shutdown_mstime`, pauses write commands, and waits.
   - Otherwise it runs `finishShutdown`.
4. **The wait.** Cron finishes the shutdown when the deadline passes or when replicas have caught up. Reads are served and connections are accepted during the wait.
5. **`finishShutdown`.**
   - Saves the RDB when save points exist and `NOSAVE` is absent, or when `SAVE` is set. A save failure without `FORCE` cancels the shutdown and the server keeps running.
   - Closes the listening sockets, then `exit(0)`. Clients are never sent a shutdown error; their sockets close when the process exits.
6. **Abort.** `cancelShutdown` unpauses, clears the deadline, and replies `-ERR Errors trying to SHUTDOWN. Check logs.` to every blocked `SHUTDOWN` caller.
7. **`INFO server`** adds `shutdown_in_milliseconds:<shutdown_mstime - now>` only while a shutdown is initiated.

### emb today (`origin/main`)

- **Signals.** `cmd/emb/main.go` registers `signal.Notify(SIGINT, SIGTERM)` only after models load. A signal during loading kills the process with Go's default action.
- **Shutdown.** The first signal creates a 30s context and calls `Server.Shutdown`, which:
  1. calls `beginDraining` (`shuttingDown` and `stateDraining`);
  2. waits on `s.active`;
  3. runs the snapshot coordinator and telemetry;
  4. calls `srv.Close()`.
- **Errors sent during shutdown.** `admitInference` (`server.go`) and the EVAL path (`script.go`) reply `ERR server shutting down`. The redcon accept callback rejects new connections by accepting and then closing them.
- **redcon constraint.** emb's redcon fork closes every connection when `Accept` returns an error, in the deferred cleanup in `serve()`. Closing the listener therefore drops live connections, which is why emb keeps accepting during drain.
- **CONFIG.** Parameter names match exactly (`p.name != name`), with underscores.

### Client behavior this aligns with

- **redis-client 0.26.4** (used by the emb Ruby gem) retries one `ConnectionError` (EOF, reset, refused) on a fresh connection when `reconnect_attempts` is set. It raises `CommandError` for RESP errors.
- **The emb gem's `ConnectionRouter`** moves to the next instance only on `CannotConnectError`, which is a refused connection.
- **Other clients** follow the same split: connection errors are retryable, RESP errors are surfaced. [INFERENCE: not checked per client in this change.]

## Goals / Non-Goals

**Goals:**
- A Redis client sees the same thing stopping emb as stopping a standalone Redis: no RESP errors, a closed or refused connection, the same `SHUTDOWN`/`ABORT` replies, and the same signal semantics.
- Operators use the Redis knobs (`shutdown-timeout`, `shutdown-on-sigterm`, `SHUTDOWN NOW NOSAVE FORCE`) with their Redis meaning.
- Deployers behind load balancers get a native lame-duck period.

**Non-Goals:**
- `-LOADING` replies during startup (Decision 9).
- Pausing writes during the grace (Decision 8).
- Shutdown aborting on snapshot failure (Decision 7).
- Changing the 30s drain deadline, the snapshot format, or client gems.
- ACLs. Protection is the password gate plus loopback-only without a password (Decision 10).

## Decisions

### Fidelity matrix

| Redis behavior | emb behavior | Why |
|---|---|---|
| SIGTERM/SIGINT run the `SHUTDOWN` sequence with `shutdown-on-sig*` flags | Same | — |
| Second SIGINT: `You insist... exiting now.`, exit 1 | Same | — |
| Second SIGTERM: no effect | Same | — |
| Signal during loading: shutdown with `NOSAVE` | Exit 0 at the next startup step, no snapshot | emb is not listening yet; same outcome |
| `SHUTDOWN` flag parsing and `-ERR syntax error` | Same, including `FORCE` | Tools send `SHUTDOWN NOW NOSAVE FORCE` |
| Caller blocks; no reply on success | Same | — |
| Extra `SHUTDOWN` during the wait blocks; flags ignored | Same | — |
| `ABORT`: `+OK` / `-ERR No shutdown in progress.` | Same text | — |
| Aborted callers: `-ERR Errors trying to SHUTDOWN. Check logs.` | Same text | — |
| Wait up to `shutdown-timeout` while replicas lag | Wait up to `shutdown_timeout` while non-`SHUTDOWN` clients are connected | emb has no replicas; its lagging party is a client still routed to it |
| Default `shutdown-timeout 10` | Default `0` | Decision 6 |
| Writes paused during the wait | Nothing paused | Decision 8 |
| Readiness during the wait: n/a | `EMB.READY` replies `draining` | emb-only command; lets probe-based routing leave first |
| Save failure keeps the server running unless `FORCE` | Shutdown always continues; `FORCE` accepted | Decision 7 |
| Listener closed just before exit | Listener closed when clients are stopped (after the grace) | Decision 3 |
| No shutdown error replies; sockets close at exit | No shutdown error replies; connections close once their running command replies | Decision 2 |
| `INFO` `shutdown_in_milliseconds` during the wait | Same field, `# Server` section | — |
| `CONFIG SET shutdown-timeout` (hyphens, integer seconds) | Accepted; canonical name `shutdown_timeout`, integer seconds | Decision 11 |
| Unauthenticated `SHUTDOWN` with protected mode off | `-DENIED` unless the client is loopback | Decision 10 |
| `-LOADING` while loading | Not listening until ready | Decision 9 |

### 1. One request path, driven by a signal goroutine
- Right after config parsing, before models load, one goroutine owns `signal.Notify(SIGINT, SIGTERM)`. It turns each signal into a `shutdownRequest{source, flags}` using the parsed `shutdown_on_*` flags.
- The `SHUTDOWN` handler sends requests to the same channel.
- The goroutine also enforces Redis's second-signal rule. If a SIGINT arrives while a request is pending or a grace runs, it logs `You insist... exiting now.` and calls `os.Exit(1)`. Native cleanup is skipped, which is safe because exit reclaims the resources, as in Redis. A repeated SIGTERM is logged and ignored.
- Before the listener opens, model preload and snapshot restore check the request channel between steps. If a request is pending, startup unwinds and exits 0 without a snapshot, because nothing was served.
- Rejected: keeping `signal.Notify` after loading. A SIGTERM during a long preload would kill the process with status 143 instead of a clean exit 0.

### 2. Stop clients without error replies
- A connection registry tracks clients:
  - a mutex-guarded set, filled from redcon's accept callback and emptied from its closed callback;
  - an atomic in-progress counter on `connState`, incremented around `mux.ServeRESP` for every command.
- `stopClients`:
  1. flips admission under `admissionMu`, as `beginDraining` does today;
  2. closes every registered connection whose counter is 0;
  3. marks the rest so that the deferred release closes them right after the reply is written.
- A command dispatched after the flip is dropped: the handler closes the connection without writing. This replaces both `ERR server shutting down` sites.
- Pipelined commands behind a running one get no reply. redcon may already have parsed them, but each later dispatch hits the post-flip check and is dropped. That matches Redis, where unprocessed input is lost at exit.
- Rejected: keeping the RESP error, the source of caller-visible failures.
- Rejected: sending `-ERR` before closing. No Redis client expects it, and they would surface it.

### 3. Refuse new connections without dropping live ones
- emb passes redcon a `gateListener` that wraps the real `net.Listener`.
- `Close()` on the gate closes the real socket, so the OS refuses new connects, the same `ECONNREFUSED` a Redis client sees after `closeListeningSockets`.
- The gate's `Accept` does not return the resulting error. It blocks until `Release()`, then returns `net.ErrClosed`.
- `Shutdown` calls `gate.Close()` at `stopClients` and `gate.Release()` just before `srv.Close()`. redcon's deferred close-all therefore runs only once every connection has finished.
- Rejected: changing the redcon fork. The wrapper keeps the dependency untouched.
- Rejected: accept-then-close (today). Multi-instance clients read it as EOF after connecting, not as a refusal, so the emb gem's `ConnectionRouter` would not fail over to another instance.

### 4. Grace period as Redis's wait
- The grace runs when all of these hold: `NOW` is absent, `shutdown_timeout > 0`, and the connection count excluding blocked `SHUTDOWN` callers is above 0.
- It sets `graceDeadline`, sets `stateDraining` (for `EMB.READY` only; `shuttingDown` stays false, so admission and accept continue), and logs `Waiting for clients before shutting down.`.
- A 100ms ticker, Redis's default cron period, ends the grace at the deadline or when the count reaches 0.
- `shutdown_in_milliseconds` reads `graceDeadline`.
- The 30s drain context is created only after the grace, so the grace never eats into in-flight or snapshot time.
- Rejected: refusing connections during the grace. LB propagation lag would turn into connection errors, and Redis keeps accepting during its wait.

### 5. `SHUTDOWN` caller blocking and abort
- The handler validates flags exactly like `shutdownCommand`, sends the request, and blocks its connection goroutine on a per-request `done` channel. redcon gives each connection its own goroutine.
- When a shutdown is already running, the extra request attaches its caller to the running request's waiter list and is otherwise ignored.
- The connection is counted as a blocked caller, so it is excluded from the grace count and is never closed as idle.
- **Abort:** `ABORT` cancels the grace. It clears `graceDeadline`, restores `stateReady`, replies `-ERR Errors trying to SHUTDOWN. Check logs.` to each waiter, and logs `Shutdown manually aborted.`.
- **Success:** waiters never get a reply; their connections close in `srv.Close()`.
- `ABORT` after clients are stopped is a command that is no longer executed: its connection closes without a reply, which is what Redis does once it is inside `finishShutdown`.

### 6. Default `shutdown_timeout` is `0`
- Redis's default of 10 applies only when replicas lag, which a standalone Redis never has, so standalone Redis stops immediately.
- emb's wait condition, clients connected, holds almost always. A default of 10 would therefore slow every Ctrl+C and container stop.
- `0` gives the same observable timing as standalone Redis. Deployers behind load balancers opt in.

### 7. Snapshot failure never blocks exit
- Redis keeps running after a failed save unless `FORCE` is set, to avoid losing data.
- emb's snapshot is a rebuildable cache, and the existing "Shutdown timeout" requirement makes its failure non-fatal. Following Redis would leave orchestrated tasks running until SIGKILL.
- emb behaves as if `FORCE` were always set, and accepts `FORCE` so the Redis idiom parses.

### 8. No write pause
- Redis pauses writes during its wait so replicas converge on the final dataset.
- emb has no replicas, and its final snapshot is taken after the drain, so a pause would only block clients for no benefit.
- Every command runs normally during the grace.

### 9. No `-LOADING`
- Redis listens while loading and answers `-LOADING`.
- emb opens its listener only once models and the snapshot are ready. TCP health checks (for example an NLB) rely on that.
- Clients see a refused connection during startup, which they retry like any connection error.

### 10. `SHUTDOWN` protection
- With a password configured, the existing auth gate applies.
- Without one, the handler parses the host of `conn.RemoteAddr()` and allows only `IsLoopback()` hosts. Others get `-DENIED SHUTDOWN is only accepted from loopback clients when no password is configured`.
- This mirrors Redis protected mode's `-DENIED`, and emb's existing rule that setting a first password requires a loopback listener.
- Rejected: unrestricted access. Any client on the network could stop a password-less server.

### 11. Redis spellings in CONFIG
- `CONFIG GET` and `CONFIG SET` normalize `-` to `_` in the requested name or pattern before matching. Replies keep canonical underscore names.
- `shutdown_timeout` is integer seconds, the Redis unit, rather than emb's usual Go durations, so `CONFIG SET shutdown-timeout 10` means the same thing on both servers.

### 12. Logging mirrors Redis
The log lines are:
- `Received SIGTERM scheduling shutdown...` (or SIGINT);
- `User requested shutdown...`;
- `Waiting for clients before shutting down.`;
- `Shutdown manually aborted.`;
- `You insist... exiting now.`;
- `emb is now ready to exit, bye bye...`.

They are not part of the spec, so wording can change without breaking clients.

## Risks / Trade-offs

- [Closing redcon connections from another goroutine while their reader blocks] → A closed `net.Conn` makes the reader return an error and fire the closed callback. Tests cover idle close, close-after-reply, pipelined drop, and abort, under `-race`.
- [`gateListener` must never let redcon see an `Accept` error before `Release`, or every connection drops] → A unit test asserts that a live connection survives `gate.Close()` until `Release()`.
- [**Breaking:** callers that matched `ERR server shutting down` now see a closed connection] → Called out in the release notes. The documented replacement is Redis's behavior.
- [The stop budget grows: `shutdown_timeout` + 30s drain + snapshot must fit the orchestrator's stop timeout, which ECS on Fargate caps at 120s] → Documented in `docs/operations.md`.
- [Long-lived client pools keep the grace running to its deadline unless something resets connections] → Intended. The deadline bounds it, and `EMB.READY` `draining` plus LB deregistration are how clients move away.
- [Second-SIGINT `os.Exit(1)` skips native cleanup] → Same as Redis's `exit(1)`. The OS reclaims memory, and a snapshot in progress leaves its temporary file, as Redis removes its temporary RDB. Snapshot writes already use a temporary file and a rename, so the previous snapshot survives.

## Migration Plan

- Ship in the next emb release.
- Release notes state the breaking change (no more `ERR server shutting down`) and the new knobs.
- `unsplash/infra`: once `emb-stable` points at that release, drop `local.emb_prestop_hold` and add `-shutdown-timeout 15` to the `emb`, `emb_canary`, and `emb_background` commands.
- Rollback: deploy the previous release; nothing persistent changes.
