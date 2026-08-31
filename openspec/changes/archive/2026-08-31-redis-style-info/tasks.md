## 1. Section builder

- [x] 1.1 Implement pure `buildInfoSections(sections []string, stats ServerStats, cache CacheStats, models []ModelInfo) string` producing `# Section\nkey:value\n...` groups in fixed order `# Server`, `# Cache`, `# Keyspace`, `# Stats`, `# Clients`, with `\r\n` endings and Redis-style trailing blank line; verify a unit test asserts exact strings for all-sections, single-section, and unknown-section (empty body) cases
- [x] 1.2 Add `cacheHitRate(hits, misses int64) string` helper (`%.1f%%`, `0.0%` when hits+misses == 0); verify unit tests cover 90/10 → `90.0%` and zero-activity → `0.0%`
- [x] 1.3 `# Keyspace` per-model lines `db0:model=<name>,keys=<entries>,hits=<hits>,misses=<misses>,hit_rate=<rate>`; verify a two-model test shows distinct per-model rates

## 2. INFO handler + wiring

- [x] 2.1 Register `info` on the mux (`mux.HandleFunc("info", s.handleINFO)`), parse `INFO [section...]` args case-insensitively (no args = all sections), reply as RESP2 bulk string; verify RESP-level test parses the bulk reply from a live conn
- [x] 2.2 Feed `ServerStats` (uptime, total_requests, total_tokens, total_errors, models_loaded, active_requests) and `CacheStats` (hits, misses, hit_rate, evictions, entries, max_bytes, memory_bytes) into the builder; verify `INFO cache` output matches the values `EMB.INFO/EMB.STATS` report for the same moment
- [x] 2.3 Add `info` to the auth-exempt command set alongside `PING`/`EMB.READY`; verify pre-auth `INFO` succeeds on a password-protected server
- [x] 2.4 Wire `redis_version:`/`emb_version:` from the injected build version (`Version` field, default `dev`); verify `INFO server` shows the injected value (or `dev` when unset)
- [x] 2.5 Add the `INFO <section...>` line to `EMB.HELP`; verify help test includes it

## 3. CONFIG GET/SET

- [x] 3.1 Add a `RuntimeConfig` registry (param → get/set hooks, RWLock) initialized from boot config, retaining raw strings; `CONFIG GET [glob]` iterates it and replies with a flat RESP array (glob via `path.Match`); verify unit tests: no-arg (all params incl. read-only), `cache*` glob, unmatched → empty array
- [x] 3.2 Register `config` on the mux and dispatch `CONFIG GET/SET`; verify RESP-level tests for both subcommands and malformed arity (`CONFIG` with no args → error)
- [x] 3.3 Implement `Cache.SetMaxBytes(n)` with immediate LRU-tail eviction; wire `CONFIG SET cache <v>` through the existing `parseCacheConfig` (auto/`N%`/size/errors identical to boot); verify tests: live shrink evicts to budget, `auto` recomputes to ≈13% RAM, `150%`/garbage rejected with the budget unchanged
- [x] 3.4 Wire `CONFIG SET password <v>` as a field swap under the mutex (takes effect for subsequent AUTH; existing sessions unaffected); wire `cache_file`/`cache_save` as validated string stores; verify tests: auth gate uses the new password, prior conns stay authenticated
- [x] 3.5 Read-only params (`listen`, `tls_cert`, `tls_key`, `models`): GET reports them, SET errors naming the parameter; verify test
- [x] 3.6 Auth posture: `config` stays OUT of the auth-exempt list (`isExempt`); verify pre-auth `CONFIG GET` → `NOAUTH` on a password-protected server
- [x] 3.7 `EMB.HELP` lists `CONFIG GET` and `CONFIG SET`; verify help test includes both

## 4. Validation

- [x] 4.1 Full suite green: `go test ./internal/server/` and the Ruby client suite (`just all`) pass with the new commands; pre-existing `TestAsyncTokenizerOverlapsWork` flake noted separately if it fails
- [x] 4.2 Smoke check with `redis-cli`/raw socket: `INFO`, `INFO server`, `INFO cache stats`, `INFO bogus` return sectioned output, filtered output, and empty body respectively; `CONFIG GET cache*`, `CONFIG SET cache 100MB` (watch `INFO` `# Cache` budget + evictions), `CONFIG SET listen :1` → read-only error; `EMB.HELP` lists `INFO` and `CONFIG`
- [x] 4.3 Supersede and remove the `emb-version` change (version discovery now belongs to `INFO`): deleted the change directory and purged its references from this change's artifacts