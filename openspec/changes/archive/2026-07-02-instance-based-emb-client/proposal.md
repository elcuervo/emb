# Instance-based Emb Client

## Motivation

The `emb` Ruby client currently only supports a global singleton (`Emb.setup`, `Emb.models`, etc.).
This makes it impossible to connect to multiple emb servers or use different configurations
in the same process. Additionally, the `EMB.MULTI` response is returned as raw binary strings
while the regular `EMB` command auto-decodes floats — an inconsistency that should be fixed.

## Scope

Refactor `Emb::Client` into an instantiable class while preserving the global convenience API.
The configuration shifts to URL-based (with `EMB_URL` env var default) while keeping backward
compatibility with `host:`/`port:` kwargs. Fix `MultiProxy` to unpack float32 binary.

## Key Decisions

1. **`Emb.new(url)` creates a fresh client** — independent pool, independent proxy registry.
2. **Global API delegates to a default client** — `Emb.setup` configures it, lazy init otherwise.
3. **URL or host/port** — accept `url:` like RedisClient, fall back to `host:`+`port:` if absent.
   Final fallback: `ENV["EMB_URL"] || "redis://localhost:6379"`.
4. **Pool size** — always a separate kwarg (`pool:`), not embedded in URL.
5. **Multi unpack** — `EMB.MULTI` response goes through the same `unpack("e*")` as single embeddings.

## Out of Scope

- TLS/SSL support (not currently in the server)
- RESP3 protocol (server uses redcon which speaks RESP2)
- Authentication (server doesn't support AUTH yet)
