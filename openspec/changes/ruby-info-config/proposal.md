## Why

The server just shipped Redis-style `INFO` (sectioned, with cache hit ratios) and `CONFIG GET`/`CONFIG SET` — but the Ruby client hasn't caught up: `Emb.stats` still returns the raw RESP **array** (`["uptime_secs", 3, "total_requests", 0, "active_requests", "0", …]`) with mixed string/int values, and there are no wrappers for `INFO` or `CONFIG` at all. Operators using the gem can't see the server's version, cache hit ratio, or live-tune the cache (`CONFIG SET cache`) without hand-rolling RESP calls. This change brings the gem's observable surface in line with the server: a typed, hash-based `stats`, a parsed sectioned `INFO`, and hot config read/change.

## What Changes

- **`Emb.stats` / `Client#stats` return a typed Hash (BREAKING).** `EMB.STATS`' flat RESP array becomes `{uptime_secs: 3, total_requests: 0, active_requests: 0, total_tokens: 0, total_errors: 0, models_loaded: 1, per_model: "…", cache_hits: 0, cache_misses: 0, cache_evictions: 0}` — symbol keys, known numeric fields coerced to Integer (including the server's string-typed `active_requests`), `per_model` stays a String (its ad-hoc text grammar is wrapper-free by design; a structured per-model breakdown is a deferrable follow-up).
- **`Emb.server_info(*sections)` / `Client#server_info` — the new `INFO` command.** Sends `INFO [sections...]`, parses the Redis section format into a nested Hash `{Server: {redis_version: "0.2.4", …}, Cache: {cache_hits: 0, cache_hit_rate: "0.0%", …}, …}` with known numeric keys coerced and `emb_version`/`redis_version` left as Strings. `server_info` (not `info`) avoids colliding with the existing `Emb.info(model)` → `EMB.INFO <model>` wrapper.
- **`Emb.config_get(*patterns)` / `Client#config_get` — hot config read.** Sends `CONFIG GET [glob]`, returns `{cache: "auto", cache_file: "", …}` (String values — they are config text, not metrics). No pattern → all params.
- **`Emb.config_set(param, value)` / `Client#config_set` — hot config change.** Sends `CONFIG SET`, returns `true` on success; raises on error (unknown/read-only param, invalid value, or `NOAUTH` on a password-protected server — server semantics surface as exceptions, nothing is silently swallowed).
- **Module-level delegation** for all four (`Emb.stats` exists and changes shape; `server_info`, `config_get`, `config_set` are new).
- **Docs:** gem README gains a "Server info & config" section (typed stats example, INFO sections, CONFIG usage, and the BREAKING note on `stats`).
- **No server changes** — the client adapts to the shipped protocol; the parser tolerates string-typed numerics so a future server fix (EMB.STATS `active_requests` int) stays compatible.

## Capabilities

### New Capabilities

- `ruby-info-config`: Ruby client wrappers for the server's `INFO` and `CONFIG GET/SET` commands, plus typed hash-based `EMB.STATS`.

### Modified Capabilities

- `emb-ruby-client` (**BREAKING**): the "Server stats" command-wrapper scenario changes — `Emb.stats`/`Client#stats` now return a typed Hash instead of the raw parsed RESP array; the command wrappers gain `server_info`, `config_get`, `config_set`.

## Impact

- **Code:** `gems/emb/lib/emb/client.rb` (+`config` memoized accessor), new `gems/emb/lib/emb/commands.rb` (stats/server_info module), new `gems/emb/lib/emb/runtime_config.rb` (`RuntimeConfig` view), `gems/emb/lib/emb.rb` (module delegation; `config` alias for `setup` removed), `gems/emb/spec/*` (unit + integration specs), `gems/emb/README.md`.
- **BREAKING (two):** `Emb.stats`/`Client#stats` return type changes array → Hash (callers doing `each_slice(2)`/`stats[0]` break); `Emb.config` is no longer an alias for `Emb.setup` (it is now the server-config view — callers using `Emb.config { |c| … }` migrate to `Emb.setup { |c| … }`). Mitigation: semver bump in the next gem release; README migration note.
- **Overlap with `ruby-client-observability` (active, unstarted):** that change's tasks 2.1–2.3 plan typed `info`/`stats` (`Emb::Types`, `Client#coerce`). Recommended sequencing: implement **this** change first (it owns typed stats + INFO/CONFIG wrappers); when `ruby-client-observability` is implemented, its typed-stats tasks are trimmed to logging/metrics/bench scope (its logger, metrics ring-buffer, and JSON/CSV bench output are unaffected and stay its own).
- **Systems:** gem only; RESP2 protocol untouched; server features already shipped and validated.
- **Auth:** `CONFIG` requires auth on password-protected servers — errors propagate as exceptions (no credential handling added).