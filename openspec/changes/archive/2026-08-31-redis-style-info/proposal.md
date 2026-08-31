## Why

Redis tooling (redis-cli, redis-py, dashboards) has no way to see *what* emb is and *how* it's doing: today only `EMB.INFO <model>` (per-model) and `EMB.STATS` (raw key/value array) exist, and neither speaks the sectioned `# Section` format Redis consumers understand. Operators want a Redis-style `INFO` that shows the build version, overall metrics, and — above all — the **cache hit ratio** for cached embeddings at a glance. This change makes `INFO` a first-class, sectioned overview that carries the build version (via `SetVersion`) plus the recently-delivered cache sizing metrics.

## What Changes

- New plain **`INFO [section ...]`** command (RESP2 bulk string, Redis section format). No args = all sections; named sections select (`INFO server`, `INFO cache stats`). Sections:
  - `# Server` — `redis_version:` (build version), `emb_version:` alias, `uptime_secs`, `process_id` (real); **no invented Redis fields** — only measurable values are reported.
  - `# Cache` (the headline section) — `cache_hits`, `cache_misses`, `cache_hit_rate`, `cache_evictions`, `cache_entries`, `cache_max_bytes`, `cache_memory_bytes`. Hit rate is a computed ratio of the cached-embedding counters.
  - `# Keyspace` — Redis-style per-model lines (`db0:keys=…,hits=…,misses=…,hit_rate=…`) so multi-model hit rates are visible per model.
  - `# Stats` — `total_requests`, `total_tokens`, `total_errors`, `models_loaded`.
  - `# Clients` — `active_requests` (real counter; no fake `connected_clients`).
- **Supersedes `emb-version`:** the separate `emb-version` change (proposed earlier) is now **removed** — version discovery over the wire is fully covered here via `INFO`'s `# Server` section and `SetVersion`. No standalone `EMB.VERSION` command is planned.
- **Auth**: `INFO` is exempt from password auth, like `PING`/`EMB.READY`, so probes identify the build and hit rates before authenticating.
- **New `CONFIG GET` / `CONFIG SET` (see-and-edit server config over Redis).** Redis-style commands for the server's runtime-settable parameters: `CONFIG GET [pattern]` returns a flat array of `param value` pairs (glob-matched); `CONFIG SET <param> <value>` validates and applies immediately. Runtime-editable v1 set: `cache` (byte budget — resizes live, evicting immediately, `auto`/`N%` recomputed at set time), `password` (applies to subsequent auth; existing connections keep their session), `cache_file` and `cache_save` (snapshot params — take effect at the next save; editable regardless of whether the `cache-snapshot` change has landed, since the save loop re-reads config each tick). Read-only params (`listen`, `tls_cert`, `tls_key`, `models`) are reported by `CONFIG GET` but rejected by `CONFIG SET` (restart-only). `CONFIG` itself SHALL **require auth** when a password is set (unlike `INFO` — it's a control channel, mirroring Redis). `CONFIG REWRITE` is a non-goal (rewriting the YAML config file destroys comments).
- `EMB.HELP` documents `INFO` and `CONFIG GET/SET`.
- `EMB.INFO <model>` and `EMB.STATS` are **unchanged** (per-model internals / raw typed array) — `INFO` is the Redis-compatible global overview, not a replacement.

## Capabilities

### New Capabilities

- `redis-style-info`: sectioned Redis-format `INFO` covering version, global stats, and cache hit ratios (global + per-model).
- `redis-config-command`: Redis-style `CONFIG GET` / `CONFIG SET` over RESP2 for runtime-settable server parameters.

### Modified Capabilities

- `emb-cmds`: `INFO` joins the command set and is documented in `EMB.HELP`.

## Impact

- **Code:** `internal/server/server.go` (new `info` handler + section builder; `config` handler + runtime-config registry; `Version` field + `SetVersion`), `internal/server/cache.go` (`SetMaxBytes` with immediate eviction), `internal/server/*test.go` (RESP parsing tests), `cmd/emb/main.go` (inject build version).
- **Interplay:** the separate `emb-version` proposal was superseded by this change (version discovery via `INFO`) and has been removed from the change board.
- **Interplay with `cache-snapshot`:** `cache_file`/`cache_save` are editable here at the *config surface* level; the snapshot change consumes them by re-reading runtime config on each save tick — no coupling, either order works.
- **APIs:** RESP2 only; `INFO [section...]` and `CONFIG GET/SET` mirror Redis semantics (empty body for unknown `INFO` sections; empty array for unmatched `CONFIG GET`; errors for unknown/read-only `CONFIG SET` params). No client gem changes required; Redis clients parse them natively.
- **Systems:** server only; no protocol breaks — `EMB.INFO`/`EMB.STATS` untouched.