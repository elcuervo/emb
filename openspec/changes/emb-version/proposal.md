## Why

The server knows its version (`cmd/emb/main.go` `var version`, injected via `-ldflags -X main.version=…` and printed by `-version`), but there is **no way to discover it over the wire**. Operators and the Ruby client can't confirm which build is running after a deploy, a canary, or a scaled-out fleet — the only checks are `PING`/`EMB.READY`, which say the server is alive, not *which* version is alive. A RESP2 `EMB.VERSION` command closes that gap for minimal cost.

## What Changes

- New RESP2 command **`EMB.VERSION`** (no args) returning a bulk-string with the server's build version (e.g. `0.2.3`). Works through any Redis client / RESP2 protocol, like the other `EMB.*` commands.
- **Redis compatibility:** plain `INFO` / `INFO server` (Redis's canonical version query — there is no `VERSION` command in Redis) SHALL also answer with a `# Server` section containing `redis_version:` plus the same version in an embedded `# emb` section. Existing Redis tooling that parses `redis_version` works unchanged.
- The server gains a `Version` field + `SetVersion(version)`, wired from `cmd/emb` (the `-X main.version` value), defaulting to `dev` when unset (matches the existing `-version` default).
- `EMB.VERSION` and `INFO` are exempt from password auth (like `PING`/`EMB.READY`) so probes can identify the build before authenticating.
- `EMB.HELP` documents both `EMB.VERSION` and `INFO`.
- The Ruby client gains `Emb.version` / `Client#version` returning the string.

## Capabilities

### New Capabilities

- `emb-version`: expose and validate the running server version over RESP2.

### Modified Capabilities

- `emb-cmds`: `EMB.VERSION` joins the command set; `EMB.HELP` documents it.

## Impact

Files: `internal/server/server.go` (Version field, `SetVersion`, handler, mux, `isExempt`, HELP line), `cmd/emb/main.go` (pass version to server), `gems/emb/lib/emb/client.rb` + `emb.rb` (client `version`), `gems/emb` specs/README, `internal/server/server_test.go`, spec delta. No protocol-format changes; additive command.

## Validation

- `EMB.VERSION` returns a bulk string equal to the `-X main.version` value (`0.2.3` from the build).
- Wrong arity (`EMB.VERSION x`) returns an error.
- On a password-protected server, `EMB.VERSION` succeeds before `AUTH`.
- `EMB.HELP` lists `EMB.VERSION`.
- Ruby `client.version` returns the same string; `just test`, `bundle exec rspec`, `just lint` all pass.