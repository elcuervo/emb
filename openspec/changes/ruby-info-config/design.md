## Context

See proposal.md — Why. Server side is shipped and validated (commit `aa91745`): sectioned `INFO`, `CONFIG GET/SET`, and the `EMB.STATS` array the client must reshape. Client today (`gems/emb/lib/emb/client.rb`): `stats` returns the raw RESP array; no INFO/CONFIG wrappers; `emb.rb` delegates module-level wrappers. `Emb.config` is already taken (alias for `setup`), so CONFIG accessors need distinct names (`config_get`/`config_set`).

## Goals / Non-Goals

**Goals**
- Typed, hash-based `stats` (the user's stated shape, with `active_requests: 0` as an Integer despite the server's string `"0"`).
- Full server-info and hot-config parity in the gem: every metric and parameter the server reports is reachable typed.
- Errors propagate loudly (CONFIG auth/validation failures raise; nothing swallowed).

**Non-Goals**
- No server modifications (the client adapts to the shipped protocol; the `EMB.STATS` `active_requests` string quirk is handled client-side by coercion rather than a server fix).
- No structured `per_model` parsing (its text grammar from `EMB.STATS` is ad-hoc; a proper structured breakdown would need a server change — deferrable follow-up).
- No credential/URI changes; `CONFIG` on password-protected servers surfaces `NOAUTH` as an exception.
- No client-side metrics/logging (that belongs to `ruby-client-observability`).

## Decisions

### 1. No type layer — values pass through the RESP decoder untouched

`redis_client` already decodes RESP integers (`:3`) as Ruby Integer and everything else as String. The client's job is only **shaping**: flat pairs → Hash, INFO text → nested sections. No coercion table, no `to_i`, no field vocabulary — `EMB.STATS`'s string-typed `active_requests` ("0") stays a String exactly as sent. If typed values are ever wanted, the home for that is the server emitting integers (its `EMB.STATS` `active_requests` TODO), not client-side guessing.

**Why no table:** a field vocabulary duplicated in the client is drift-prone (the server is the source of truth for its own fields) and buys nothing — the decoder already types RESP. Simplicity wins; the ask is "a nice hash", not a type layer.

### 2. `stats` is a flat `to_h` over the decoded pairs

`EMB.STATS` returns a flat `key, value, key, value…` array. Parse with `each_slice(2)` and symbolize keys. No nesting — matches the user's example shape exactly (values decode to Integer where the server sends ints, String otherwise):

```ruby
{uptime_secs: 3, total_requests: 0, active_requests: "0", total_tokens: 0,
 total_errors: 0, models_loaded: 1, per_model: "…", cache_hits: 0, …}
```

**Why not section-nesting stats:** `EMB.STATS` has no `# Name` markers (it's a flat kv array, unlike `INFO`); nesting would invent structure the protocol doesn't carry. `server_info` provides the sectioned view instead.

### 3. `server_info` parses the Redis section grammar into a nested hash

`INFO` reply is text: `# SectionName` headers, `key:value` lines, blank-line separators. Parser: split lines, on `# <name>` open a new top-level key (`:Server` → Symbol), otherwise split on first `:` and store the decoded value as-is. `server_info(*sections)` maps `sections` to `INFO <sections...>` args (strings or symbols both accepted, downcased to match the server's case-insensitive matching); **no args → `INFO` with no sections = all sections** (the default).

**Why the name `server_info`:** it is the server-wide counterpart to `Emb.info(name)` (which is taken by `EMB.INFO <model>`); `server_info` names the Redis-compat `INFO` precisely and never ambiguates with per-model info. Bare `server_info` reads as "the whole server's info", matching the all-sections default.

### 4. `config` is a Hash-like `RuntimeConfig` view — not method pairs

`Emb::RuntimeConfig` (one file, included via `Client#config`) exposes the server's runtime config the way a Ruby settings object should: `to_h` (all params), `config[key]` (read one — exact key → String value, glob → Hash, unknown → `nil`), `config[key] = value` (write — the assignment expression yields the RHS per Ruby setter semantics; the effect is verified by a subsequent read). `Emb.config` stops aliasing `Emb.setup` and becomes this view; `Emb.setup` is the one way to configure the client.

**Why a Hash-like view over `config_get`/`config_set` methods:** it wins on all three axes the methods lost — no boolean-return method (`[key]=` returns the server reply, so no `Naming/PredicateMethod` override is needed), no client-class bloat (views are a separate object, so no `ClassLength` bump either), and a familiar surface (`config['cache'] = '100MB'` reads like `ENV['X'] = …`).

**Why `[]` returns a Hash for globs but a String for exact keys:** exact-key reads are the common case and should be scalar; globs are a power-user escape hatch where multiple matches must all surface. `nil` for unknown keys matches `Hash#[]` semantics.

**Why no value coercion in config:** a config String is a round-trippable store (`config['cache']` feeds straight back into `config['cache'] =`); coercion would corrupt that contract. Metrics decode; config doesn't.

### 5. Ordering and naming of module delegates

`emb.rb` adds `server_info`, `config_get`, `config_set` (and `stats` keeps delegating, now returning the hash). `Emb.config` remains `setup` (unchanged); the new `config_get`/`config_set` are explicitly named to avoid colliding.

## Risks / Trade-offs

- **BREAKING `Emb.stats` shape** → semver-major or minor-with-note per gem's release convention; README migration note; specs updated in the same commit (no silent change).
- **Server sends `active_requests` as `"0"` string (EMB.STATS TODO)** → passed through as-is, per the no-type-layer decision; the real fix is the server emitting an integer (its TODO), which any client benefits from.
- **INFO text grammar is external** (server-controlled) → parser is strict-but-tolerant: unknown lines ignored rather than raising, so forward-compatible server sections don't break the client (unknown keys land as String values).
- **Section order instability** → the parser keys by section name, not position, so order never matters; unknown section names land as their own keys.
- **Overlap with `ruby-client-observability` typed-stats tasks** → sequencing decided in proposal: this change owns stats/INFO/CONFIG wrappers and the hash shapes; `ruby-client-observability`'s typed `info`/`stats` (if still wanted) builds from here.

## Migration Plan

- Ship in the next `emb` gem release with the BREAKING note; existing `stats` callers migrate from `arr.each_slice(2).to_h…` to the hash directly.
- Nothing server-side; no flags; no data migration.
- Rollback: revert gem bump (client-only).

## Open Questions

None blocking. Deferrable follow-ups (each its own change): structured `per_model` breakdown (would require an `EMB.STATS` format addition), mypy/Python or other client parity, and client-side `INFO` caching with TTL if section parsing ever shows up in hot paths.