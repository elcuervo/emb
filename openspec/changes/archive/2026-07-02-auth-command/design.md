## Context

The emb server uses `github.com/tidwall/redcon` for RESP protocol handling. Commands are registered via `redcon.NewServeMux()`. Each connection can carry state via `conn.SetContext()` / `conn.Context()`.

Currently there is no auth — all connections are fully trusted.

## Goals / Non-Goals

**Goals:**
- Match Redis `requirepass` AUTH semantics exactly
- Zero overhead when password is not set
- Per-connection auth state (no global auth, no session tokens)

**Non-Goals:**
- ACLs, user management, TLS client certs, or any multi-user auth model
- Password rotation, expiry, or management commands
- Environment variable support for password (config-only for now)

## Decisions

1. **Config key is `password`, not `requirepass`.** The YAML and flag both use `password` — simpler, self-documenting, matches what it is.

2. **Per-connection state via `conn.SetContext`.** redcon provides this mechanism. On accept we store `&connState{}`. On AUTH success we flip `authenticated`. No maps, no locking needed.

3. **Inline dispatch guard, not middleware wrapper.** The `redcon.NewServer` handler closure checks `password != "" && !isExempt(cmd) && !isAuthenticated(conn)` before delegating to the mux. Two pure helper functions (`isExempt`, `isAuthenticated`) keep the check readable. When no password is configured, the guard short-circuits — zero overhead.

4. **PING is always exempt.** Matches Redis behavior and lets load balancers / health checkers probe without credentials.

5. **Double AUTH returns OK both times.** Matches Redis. No reason to reject it.

## Risks / Trade-offs

- **Passwords in config files** — the YAML file contains the password in plaintext. Acceptable for this stage; TLS and env-var support can be added later.
- **No rate limiting on AUTH** — brute force is possible over the wire. Mitigated by the fact that this is a service embedding tool, not a user-facing auth system. If needed later, connection rate limiting is orthogonal.
