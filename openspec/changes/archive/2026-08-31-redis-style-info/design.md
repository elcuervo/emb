## Context

See proposal.md — Why. Today: only `EMB.INFO <model>` (per-model key/value array) and `EMB.STATS` (global key/value array) exist; there is no plain `info` handler (`server.go:70-78` mux list). This change adds the full sectioned `INFO`, and with it the `Version` field + `SetVersion` plumbing (the earlier `emb-version` proposal that would have injected it is superseded by this one and was removed).

## Goals / Non-Goals

**Goals**
- `INFO` speaks Redis section format so stock Redis tooling parses it.
- Real numbers only — every reported field is measured; no invented Redis fields.
- Cache hit ratio is immediately visible: its own section, plus per-model lines.

**Non-Goals**
- No change to `EMB.INFO <model>` / `EMB.STATS` semantics (they stay; `INFO` is additive).
- No new counters beyond what exists (`EMB.STATS` + cache stats already provide them all) — this change is a *view*, not new instrumentation. If a metric is missing (e.g., a real client-count), it is omitted, not faked.
- No Ruby client work (Redis clients parse `INFO` natively); the observability change's typed `info`/`stats` is orthogonal.

## Decisions

### 1. One section builder, pure string assembly

`buildInfoSections(which []string, snap ServerStats, cache CacheStats, models []ModelInfo) string` — pure function, unit-testable without a server. `# Section\nkey:value\n...` with `\r\n` endings; a trailing blank line per Redis. Unknown section names produce an empty body, not an error (Redis semantics).

**Why pure:** the RESP-parsing tests (like the existing `server_test.go` RESP helpers) can assert on exact strings without sockets; sections compose trivially later.

### 2. `# Cache` is the second section; `# Keyspace` carries per-model rates

Order fixed: `# Server`, `# Cache`, `# Keyspace`, `# Stats`, `# Clients`. The user's headline metric (`cache_hit_rate`, from `CacheStats()`) lands right after the version — greppable in any tooling. `# Keyspace` mirrors Redis's `db0:keys=...` shape as `db0:model=<name>,keys=<entries>,hits=<hits>,misses=<misses>,hit_rate=<rate>`.

**Why Keyspace-style lines instead of more sections:** Redis operators already read `db0:` lines as "per-key-space storage"; a per-model cache is an exact analogy. It also keeps `INFO` one page regardless of model count.

**Alternative considered:** a `# Models` section with one `key:value` per model index. Rejected — Redis tooling pattern-match `dbN:` prefixes less than nothing; and the `,`-separated line condenses better.

### 3. Hit ratio lives in one helper

`cacheHitRate(hits, misses) string` — `hits/(hits+misses)*100.0`, `%.1f%%`, `0.0%` when hits+misses == 0. Single source of truth shared conceptually with `EMB.INFO`'s existing `cache_hit_rate` (which today computes the same way) so the two never diverge.

**Why a helper rather than trusting callers:** division-by-zero and formatting consistency are exactly the bugs specs exist to hold (see the spec's "No requests yet" scenario).

### 4. Section selection honors broker case-insensitively? No — exact lowercase

Redis section names are case-insensitive (`INFO SERVER` works). Decision: accept exact case-insensitive match via `strings.EqualFold` — one line, matches Redis, no complexity. Unknown names still yield empty.

### 5. Auth exemption mirrors `PING`/`EMB.READY`

The mux-level auth wrapper already exempts those commands (`internal/server/server.go` auth gate); `info` joins the list. Rationale: identical probe semantics — version + read-only stats before credentials.

### 6. Version plumbing lives here now

The `Version` field + `SetVersion(version)` (default `dev`) are implemented by this change and wired from `cmd/emb/main.go`; `INFO`'s `# Server` section consumes the field. The earlier `emb-version` proposal (standalone `EMB.VERSION` command + `INFO` compat) was superseded by this change's `INFO` and removed from the board.

### 7. CONFIG GET/SET: a runtime-config registry, not a config-file editor

The server holds a `RuntimeConfig` (a small registry of `param → {get, set}` hooks) initialized from the boot config and consulted with a read/write mutex. `CONFIG GET` iterates the registry in declaration order; `CONFIG SET` dispatches to the param's setter. Raw string values are retained (not just parsed bytes) so `GET` echoes operator input (`auto`, `25%`, `1GB`).

**Why registry over editing the YAML/`config.Config` struct:** the YAML file on disk is boot-only (and rewriting it destroys comments — `CONFIG REWRITE` is a non-goal); a struct isn't thread-safe and mixes boot-only with live params. A registry with per-param setters makes "apply immediately" a small, testable unit per parameter and keeps restart-only params (`listen`, `tls_*`, `models`) naturally GET-only.

**Why a mutex rather than atomics per field:** `cache_file`/`cache_save` are strings read by the save-loop goroutine; one RWLock over the registry is simpler than N atomics, at negligible cost (config ops are rare).

### 8. Cache resize is a real runtime op: `SetMaxBytes` + immediate eviction

`Cache` gains `SetMaxBytes(n int64)`: under the existing `c.mu`, update `maxBytes`, then evict the LRU tail while `curBytes > maxBytes` (reusing the same eviction path `Set` uses). `CONFIG SET cache <v>` parses with the *existing* `parseCacheConfig` (so `auto`/`N%`/sizes/errors are identical to boot-time semantics, including the recomputed ~13%-of-RAM value) and calls `SetMaxBytes`.

**Why reuse `parseCacheConfig`:** one validator, one story — `CONFIG SET cache 150%` fails exactly like a bad boot config, and the spec's "invalid value rejected" scenario falls out of existing code.

**Edge:** `CONFIG SET cache ""` (disable) is out of scope v1 — a nil-cache transition mid-session adds nil-checks to every hot path for a case operators rarely need live; documented as restart-only.

### 9. Password is a field swap; config commands are NOT auth-exempt

The auth gate and `handleAUTH` read `s.password` at command time (`server.go:81-82,181`), so `CONFIG SET password` is a plain field swap with the mutex held; already-authenticated connections keep their session (per-session `authenticated` flag on the conn, not re-checked against the password). `config` joins the mux but stays **out** of `isExempt` — unlike probe-exempt `INFO`, it requires auth, matching Redis where `CONFIG` is a control plane.

## Risks / Trade-offs

- **Redis tools parse `redis_version:` loosely** (e.g., `redis-cli --version` clients comparing semvers) → we report the real build version there, and `emb_version:` exists precisely so emb-specific tooling keys on a clearly-named field; semver-shaped output kept (`0.2.4`, `dev`).
- **Sections can drift from `EMB.STATS`/`EMB.INFO`** → single `ServerStats` snapshot struct feeds both the existing array writers and the new section builder; one source of truth per metric.
- **Standalone `EMB.VERSION` no longer planned** → version discovery is `INFO`'s job; the superseded `emb-version` change was removed. If a dedicated `EMB.VERSION` command is ever wanted, it is a trivial additive follow-up.
- **No connection/client metrics** (Redis has a rich `# Clients`) → honest omission per non-goal; a real `active_requests` counter exists and is reported where the current code tracks it.
- **`CONFIG SET password` could lock out operators** (setting a password when none existed, mid-session) → requires auth to *issue*, matches Redis, and documented; a follow-up could add a `CONFIG GET`-visible `requirepass` hint — not v1.
- **`cache_file`/`cache_save` editable before the snapshot change lands** → harmless: config ops store values the runtime keeps; the string values travel through `EMB.INFO`/`INFO` read views and snapshot code only exists once `cache-snapshot` ships.
- **Live cache shrink vs in-flight `Set`s** → resize happens under the cache mutex and the normal eviction path; concurrent writes simply observe the new budget like any other budget state.

## Migration Plan

- Pure additive command; no config, no flags, no wire changes to existing commands.
- This change owns the version plumbing (`SetVersion`) and `INFO` output; the superseded `emb-version` change was removed from the board. Nothing else to sequence.
- Rollback: remove the `info` mux registration; everything else is unchanged.

## Open Questions

None — section set, order, per-model line format, auth exemption, and sequencing are settled. The only deferrable: whether future sections (`# Memory` with real RSS via `registry/sysmem`, `# Commandstats`) appear later as additive extensions.