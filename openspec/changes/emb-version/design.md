## Context

`version` is a `cmd/emb` package variable set by the build (`justfile` passes `-ldflags="-X main.version=0.2.3"`), surfaced only via the `-version` CLI flag. The `server` package knows nothing about it, and the RESP command surface (`EMB.*`) has no version query. Version-gated rollouts, canaries, and client compatibility checks therefore have to rely on out-of-band config. This change threads the version into the server and exposes it as a plain RESP2 bulk string.

## Goals / Non-Goals

**Goals:**
- `EMB.VERSION` → bulk string of the build version, via any RESP2 client
- Version flowed from `cmd/emb` to the server without changing the `Server` constructor signature (many tests call `server.New`)
- Exempt from password auth (health-probe friendly)
- Documented in `EMB.HELP`; surfaced in the Ruby client

**Non-Goals:**
- Version comparison/semver validation server-side (clients decide)
- Changing the RESP protocol format (still RESP2 bulk strings throughout)
- Per-model versioning

## Decisions

### `Server.Version` field + `SetVersion`, not a `New` param
`Server` keeps its constructor; `main` calls `srv.SetVersion(version)` before `ListenAndServe`. Tests set it explicitly; unset defaults to `dev` at the handler (matching the CLI `-version` default). Zero blast radius on the ~20 `server.New` call sites.

### Handler mirrors `EMB.READY` conventions
Registered as `emb.version` (redcon lowercases command names), validates exactly one arg, replies `conn.WriteBulkString(s.version)`. Arity error → `ERR wrong number of arguments for 'EMB.VERSION'`.

### Redis `INFO` compatibility
Redis has no `VERSION` command; the compatible surface is `INFO server` (parsed by redis-cli and admin/probe tooling via the `redis_version:` line). We register `info` (`INFO [server]`) → a RESP2 bulk string:
```
# Server
redis_version:0.2.3

# emb
version:0.2.3
```
`INFO` with no args or `INFO server` both return this; any other `INFO <section>` returns the same server section (categorised additively). This gives drop-in compat for existing Redis health checks that poll `INFO`.

### Exempt from auth alongside `PING`/`EMB.READY`
`isExempt` (server.go:167-173) already whitelists `auth`, `ping`, `emb.ready`; add `emb.version`. Rationale: version probes are part of LB/health discovery and must not require a secret.

### Ruby client surfaces it as `version`
`Client#version` = `send_command('EMB.VERSION')`; module-level `Emb.version` delegates to the default client. String return (already coerced by Types as passthrough — `version` is not in any typed table).

## Risks / Trade-offs

- [Default `dev` in non-injected builds] → matches existing `-version` behavior; document that release builds must set `-X main.version`.
- [Arg-light command could be confused with `EMB.MODELS`] → naming explicit; HELP documents it.

## Migration Plan

1. Server: `Version` field, `SetVersion`, `handleVERSION`, mux registration, `isExempt`, HELP line.
2. `cmd/emb/main.go`: `srv.SetVersion(version)`.
3. Ruby client `version` + module accessor + README; gem spec.
4. Server tests: default `dev`, set version echo, arity error, pre-auth exemption, HELP line.

## Open Questions

- Include `version` as a field in `EMB.INFO` as well? (Proposal: no — keep EMB.VERSION single-purpose; the gem can compose.)