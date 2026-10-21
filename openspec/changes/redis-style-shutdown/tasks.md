# Tasks

## 1. Configuration

- [x] 1.1 In `internal/config/config.go`, add `shutdown_timeout` (integer seconds, non-negative, default 0) and `shutdown_on_sigterm` / `shutdown_on_sigint` (strings, default `default`). Add the `-shutdown-timeout`, `-shutdown-on-sigterm`, and `-shutdown-on-sigint` flags next to `-idle-timeout`. Verify with table tests for YAML, flags, defaults, and rejection of negative or fractional timeouts.
- [x] 1.2 Add `config.ParseShutdownFlags(string) (ShutdownFlags, error)`, shared by startup, signal mapping, and `CONFIG SET`. It parses a case-insensitive, space-separated combination of `default|save|nosave|now|force`, rejects `save` with `nosave` and unknown words, and formats back to the canonical string. Verify with table tests, including round-trips such as `nosave now`.

## 2. Listener and connection tracking

- [x] 2.1 Add `gateListener` in `internal/server`:
  - `Close()` closes the real socket;
  - `Accept` blocks after `Close()` until `Release()`, then returns `net.ErrClosed`.

  Pass it to redcon in `Start` and `ListenAndServe`. Verify with a test that a live connection keeps working after `Close()`, that a new dial is refused, and that `Release()` lets `Serve` return.
- [x] 2.2 Add the connection registry: a mutex-guarded set from the accept and closed callbacks, plus an atomic in-progress counter on `connState`, held around `mux.ServeRESP` for every command. Verify with a `-race` test that the set and counter follow connect, command, and disconnect.

## 3. Shutdown sequence

- [x] 3.1 Add `stopClients`:
  - closes the gate;
  - flips admission under `admissionMu`;
  - closes registered connections whose counter is 0;
  - marks the rest to close after their reply.

  Also make the dispatch path close the connection without writing when admission has flipped, and delete both `ERR server shutting down` sites (`server.go` `admitInference`, `script.go`). Update `admission_lifecycle_test.go`. Verify with tests: an idle connection's next command gets a connection error; an in-flight `EMB` replies and then closes; a pipelined second command gets no reply; a new dial is refused; the existing drain-timeout test passes.
- [x] 3.2 Add the grace phase:
  - applies only when `NOW` is absent, `shutdown_timeout > 0`, and non-`SHUTDOWN` connections exist;
  - sets `graceDeadline` and `stateDraining` while admission stays open;
  - uses a 100ms ticker that ends at the deadline or when the count reaches 0;
  - can be cancelled by abort.

  Keep creating the 30s drain context after the grace. Apply `SAVE`/`NOSAVE` to the snapshot coordinator, and log and continue when `SAVE` has no `cache_file`. Verify with tests: commands served and `EMB.READY` `draining` during the grace; early end within 100ms of the last disconnect; bounded end; no grace with `NOW`, zero timeout, or no clients; the drain deadline is unaffected by a full grace; `SAVE` and `NOSAVE` override `cache_save_on_shutdown`.

## 4. SHUTDOWN command

- [x] 4.1 Register `shutdown`, with parsing identical to Redis `shutdownCommand`:
  - case-insensitive `nosave|save|now|force|abort`;
  - `-ERR syntax error` for unknown flags, `abort` with any other flag, and `save`+`nosave`.

  Verify with table tests on the exact reply bytes.
- [x] 4.2 Implement blocking and abort:
  - the caller sends a request and waits on its request's waiter list, excluded from the grace count and from idle close;
  - a second `SHUTDOWN` during a shutdown attaches as a waiter and its flags are ignored;
  - `ABORT` during the grace replies `+OK`, replies `-ERR Errors trying to SHUTDOWN. Check logs.` to every waiter, and restores `stateReady`;
  - `ABORT` with nothing running replies `-ERR No shutdown in progress.`.

  Verify with tests for each reply, waiter behavior, and a later full shutdown after an abort.
- [x] 4.3 Access control: with a password, use the existing auth gate; without one, allow only loopback `RemoteAddr` hosts and send other clients a `-DENIED ...` error. Verify with tests from loopback and non-loopback addresses, and with a password set.
- [x] 4.4 Document `SHUTDOWN` in `EMB.HELP`. Verify with the existing help test pattern.

## 5. Signals and entrypoint

- [x] 5.1 In `cmd/emb/main.go`, start the signal goroutine right after config parsing and before model loading. It:
  - maps SIGTERM and SIGINT through `shutdown_on_*` into requests on the shared channel;
  - exits with status 1 and logs `You insist... exiting now.` on a SIGINT while a request is pending or a grace runs;
  - logs and ignores a repeated SIGTERM.

  Move the select loop into a testable function that runs grace, then drain, then exit, and returns to waiting after an abort. Verify with tests driving fake signal and request channels: grace then shutdown; `now`; abort then a later SIGTERM; repeated SIGTERM ignored; second SIGINT calls an injected exit with 1.
- [x] 5.2 Check the request channel between preload and restore steps before the listener opens; on a pending request, unwind and exit 0 without a snapshot. Verify with a test that injects a request during startup.
- [x] 5.3 Emit the Redis-style log lines listed in design Decision 12. Verify in the 7.2 run log.

## 6. CONFIG and INFO

- [x] 6.1 Add `shutdown_timeout`, `shutdown_on_sigterm`, and `shutdown_on_sigint` to `configParams`. Setters validate with the 1.x parsers, and a change applies to the next shutdown. Verify with CONFIG GET and CONFIG SET tests, including rejected values keeping the previous value.
- [x] 6.2 Normalize `-` to `_` in `CONFIG GET` patterns and `CONFIG SET` names, and reply with canonical names. Verify with tests: `CONFIG SET shutdown-timeout 10` then `CONFIG GET shutdown-timeout` returns `shutdown_timeout` `10`; `CONFIG GET cache-save-on-shutdown` works; existing CONFIG tests pass.
- [x] 6.3 Emit `shutdown_in_milliseconds` in `INFO server` only while a grace runs (deadline minus now). Verify with tests during a grace, in normal operation, and after an abort.

## 7. Redis parity and end-to-end check

- [x] 7.1 Add `internal/server/shutdown_parity_test.go`. It is gated by `EMB_REDIS_PARITY=1` and skips when `redis-server` is not on `PATH`. It runs the same client script against `redis-server --port 0 --save ''` and against emb, and asserts identical client-visible results:
  - `SHUTDOWN SAVE NOSAVE`, `SHUTDOWN ABORT NOW`, and `SHUTDOWN LATER` replies;
  - `SHUTDOWN ABORT` with nothing running;
  - the error class (connection error, not command error) an idle connection gets after `SHUTDOWN NOW NOSAVE`;
  - a refused dial after exit;
  - exit status 1 on a double SIGINT.

  Grace, abort, and blocked-waiter text are compared only on emb: Redis waits only with lagging replicas. Verify with `nix develop --command bash -c 'EMB_REDIS_PARITY=1 go test ./internal/server/ -run Parity -count=1'`.
- [x] 7.2 Inside `nix develop`, run `just lint`, `just test`, and `just build`. Then run `./bin/emb -config test-two-models.yaml -listen 127.0.0.1:16379 -shutdown-timeout 10` with a held `redis-cli` connection and check:
  - after SIGTERM, `EMB` answers, `EMB.READY` replies `draining`, and `INFO server` shows `shutdown_in_milliseconds`;
  - `SHUTDOWN ABORT` from loopback replies `OK`, and `EMB.READY` returns `OK`;
  - after another SIGTERM, closing the held connection ends the grace at once and the process exits 0 with the Redis-style log lines;
  - a fresh start, then SIGINT twice, exits 1.

## 8. Documentation

- [x] 8.1 `docs/commands.md`: document `SHUTDOWN` with syntax, flags, replies, blocking, abort, and access control, and remove `ERR server shutting down`. `docs/operations.md`, in the Connection lifecycle section: document the shutdown sequence, the grace, refused connections, second SIGINT, the stop-timeout budget (`shutdown_timeout` + 30s drain + snapshot), and the deliberate differences from Redis. `docs/configuration.md`: add the three parameters to the table and the YAML sample, and note the hyphenated CONFIG names. Verify that every command in these docs ran as written in 7.2.
- [x] 8.2 Mirror 8.1 in `website/docs/index.html`, as the `product-docs` parity requirement demands, and add the README shutdown notes. Verify that the site's docs page lists `SHUTDOWN` and the three parameters with the same fields as `docs/commands.md` and `docs/configuration.md`.

## Workflow follow-up

- Release through the usual `just version` / `just release` flow, with release notes calling out the removed `ERR server shutting down` reply.
- Tell `unsplash/infra` which version to use; its `emb-restart-traffic-routing` follow-up replaces `local.emb_prestop_hold` with `-shutdown-timeout 15`.
- Archive this change after release.
