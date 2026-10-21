# Proposal

## Why

emb speaks RESP, so callers use ordinary Redis clients and expect a server that shuts down the way Redis does. Today it doesn't:
- On SIGTERM, emb drains at once and answers commands on existing connections with `ERR server shutting down`. Redis never sends that error. Redis clients treat connection errors as retryable and reconnect, but they surface RESP errors to the caller. So every emb restart produces errors that a Redis restart would not.
- emb has no grace period, no `SHUTDOWN` command, no `SHUTDOWN ABORT`, and no `shutdown-on-sigterm` / `shutdown-on-sigint`. It also does not treat a second Ctrl+C as "exit now".
- Deployers behind load balancers wrap emb in shell scripts that delay SIGTERM, because the server has no grace period of its own.

## What Changes

The shutdown contract follows Redis 7 (`src/server.c` `prepareForShutdown`/`finishShutdown`/`sigShutdownHandler`, `src/db.c` `shutdownCommand`), with emb analogs where Redis concepts do not exist.

- **Errors during shutdown, removed.** After the grace, commands that were not already running are not executed and get no reply, and their connection closes. Callers see a connection error, as when Redis exits. `ERR server shutting down` is no longer sent.
- **Clients stopped the way Redis does.**
  - Running commands finish and reply, then their connection closes.
  - Idle connections close immediately.
  - The listening socket closes, so new connections are refused.
- **Grace period, `shutdown-timeout` (integer seconds).** Redis waits up to this long for lagging replicas. emb waits up to this long while clients other than blocked `SHUTDOWN` callers are still connected.
  - During the grace every command is served and new connections are accepted.
  - The grace ends at the deadline or as soon as those clients are gone, checked every 100ms like Redis's default cron.
  - The default is `0`. A standalone Redis has no replicas to wait for and also stops immediately, so this preserves today's timing.
- **`SHUTDOWN [NOSAVE|SAVE] [NOW] [FORCE] [ABORT]`**, with Redis's parsing, errors, and replies:
  - `-ERR syntax error` for unknown flags, `SAVE`+`NOSAVE`, or `ABORT` combined with any other flag;
  - the caller blocks and gets no reply on success, because the connection closes;
  - a second `SHUTDOWN` during the grace also blocks;
  - `ABORT` replies `+OK` or `-ERR No shutdown in progress.`;
  - blocked callers get `-ERR Errors trying to SHUTDOWN. Check logs.` when the shutdown is aborted;
  - `FORCE` is accepted, because the snapshot is a cache and its failure never blocks exit, so the documented `SHUTDOWN NOW NOSAVE FORCE` idiom works.
- **`shutdown-on-sigterm` / `shutdown-on-sigint`** take `default`, `save`, `nosave`, `now`, and `force`, in any combination except `save` with `nosave`.
- **Signals, as in Redis:**
  - SIGTERM and SIGINT run the same sequence as `SHUTDOWN`, with their configured flags;
  - a second SIGINT while a shutdown is pending exits immediately with status 1 and no snapshot ("You insist... exiting now.");
  - a second SIGTERM changes nothing;
  - a signal during startup (models loading or snapshot restoring) exits 0 without saving.
- **`INFO server`** reports `shutdown_in_milliseconds` only while a grace runs.
- **CONFIG names.** The three parameters use emb's underscore names (`shutdown_timeout`, ...). `CONFIG GET` and `CONFIG SET` also accept Redis's hyphenated spellings for every parameter, so Redis tooling works unchanged.
- **Protection.** With a password configured, `SHUTDOWN` needs authentication like any command. Without one, only loopback clients may call it; others get a `-DENIED` error, following Redis protected mode.
- **Documentation.** `EMB.HELP`, `docs/commands.md`, `docs/operations.md`, `docs/configuration.md`, `website/docs/index.html`, and the README describe the contract.

Kept deliberately different from Redis (see design):
- No `-LOADING` replies. emb listens only once ready, so TCP readiness checks stay correct.
- No write pause during the grace. Redis pauses writes so replicas can converge; emb has no replicas, and it takes its snapshot after the drain.
- Snapshot failure never keeps the process running, as if `FORCE` were always set.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `server-lifecycle`:
  - Added: the grace, signal options, `SHUTDOWN` and `ABORT`, the second-SIGINT exit, protection, shutdown during startup, and how connections end.
  - Removed: the "ERR server shutting down" requirement.
  - Modified: the shutdown sequence and the admission-race requirement.
- `redis-config-command`: the shutdown parameters, and Redis hyphenated parameter names.
- `redis-style-info`: `shutdown_in_milliseconds`.

## Impact

- `cmd/emb/main.go`:
  - signal handling starts before model loading;
  - one shutdown-request loop serves signals and `SHUTDOWN`;
  - a second SIGINT exits immediately.
- `internal/server`:
  - the grace phase and the shutdown request channel;
  - a connection registry and per-connection in-progress tracking;
  - a closable-but-blocking listener wrapper, because redcon's serve loop closes all connections when `Accept` fails;
  - the `SHUTDOWN` handler, CONFIG parameters and name normalization, the INFO field, and `EMB.HELP`;
  - removal of both `ERR server shutting down` replies (`server.go`, `script.go`).
- `internal/config`: YAML keys and CLI flags.
- Documentation: the files listed above. `website/docs/index.html` must stay in parity with `docs/commands.md` and `docs/operations.md` (`product-docs`).
- **Breaking:** callers that matched the `ERR server shutting down` string now see a closed connection instead.
- Clients: no gem change is needed. The Ruby gem's `ConnectionRouter` already moves to another instance on a refused connection (`CannotConnectError`), and redis-client reconnects on EOF.
- Downstream: `unsplash/infra` replaces its SIGTERM-delay wrapper with `-shutdown-timeout 15` once a release containing this is deployed.
