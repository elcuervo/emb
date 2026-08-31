## Why

A restart costs the whole working set: with `bigger-memory-cache`'s `auto` sizing (now up to ~13% of RAM) a warm cache can hold hundreds of thousands of distinct-text embeddings, and every boot re-embeds them one by one (we measured ~400k distinct texts ≈ 223s of re-warming). Redis solves this for its data with RDB snapshots; emb's embedding cache has no equivalent. Dumping the cached embeddings — raw float32 bytes, already the wire format — and reloading them into memory on boot turns restarts into instant warm caches for a given model.

## What Changes

- **Snapshot format (binary, versioned).** Magic `EMBCACHE` + format version + entry count + entries of `(model, text, dim, vector bytes)` with little-endian length prefixes. Vectors are the server's raw float32 payload — no re-encoding, no float widening (keeps the float-bloat-free property).
- **`EMB.SAVE` command.** Synchronous, on-demand dump of the cache to the configured path: write `*.tmp`, flush, atomic `rename`. Reports bytes + entry counts per model in the reply. Auth-exempt (like `PING`/`EMB.READY`/`INFO`) so probes can snapshot before auth.
- **Save on graceful shutdown.** `server.Shutdown` hook (the existing signal path in `cmd/emb/main.go`) writes a snapshot when the cache is non-empty and a path is configured — so clean restarts recover the working set automatically.
- **Load on boot.** If the snapshot file exists and the cache is enabled, entries are restored after model registration. Only **loaded models** (honoring `preload: true` / models already loaded) receive entries — rows for unknown or not-yet-loaded models are skipped and counted (`cache_restore_skipped`). Restored recency order rebuilds LRU semantics (favored entries stay).
- **Config.** `cache_file: <path>` YAML key + `-cache-file` CLI flag (goes next to `cache:`; empty = persistence off).
- **Periodic snapshots (RDB-style).** `cache_save: <seconds> <changes> [...]` YAML key + `-cache-save` flag accepts Redis-style `save` pairs (e.g. `"900 1 300 10"`): a background loop saves when a pair's elapsed-seconds AND changed-entries thresholds are both met. Saves are non-blocking (reference-capture snapshot; the request path is never stalled), single-flight (coalesced like Redis's "last save failed"), log errors without failing, and a final save always runs on graceful shutdown. Multiple pairs OR no pairs (periodic off, manual `EMB.SAVE` / shutdown only) both supported.
- **Resilience.** Corrupt, truncated, or too-new-format files are skipped with a warning — the server always boots (mirrors Redis starting empty). Restored size respects the cache's `maxBytes` budget (stop loading / evict tail when over budget).
- **Metrics.** `EMB.INFO <model>` and `INFO` report restore counters: `cache_restored_entries`, `cache_restore_skipped`, plus `cache_file` path/loaded-state.

## Capabilities

### New Capabilities

- `cache-snapshot`: versioned binary snapshot of cached embeddings — `EMB.SAVE`, shutdown-save, boot-restore, and restore metrics.

### Modified Capabilities

- `lru-cache`: the cache gains snapshot dump/restore and restore-counters in `EMB.INFO`; `EMB.HELP` documents `EMB.SAVE` (via `emb-cmds`).
- `emb-cmds`: `EMB.SAVE` joins the command set and is documented in `EMB.HELP`.

## Impact

- **Code:** `internal/server/cache.go` (iteration API + reference snapshot + dirty counter), new `internal/server/snapshot.go` (format writer/reader + save loop), `internal/server/server.go` (`EMB.SAVE` handler + shutdown hook + boot restore), `internal/config/config.go` (`cache_file`, `cache_save`), `cmd/emb/main.go` (flags + shutdown invocation), tests.
- **Format stability:** a magic + format version field means future changes bump the version and older binaries skip newer files (never crash).
- **APIs:** RESP2 `EMB.SAVE`; config surface additive (`cache_file`, `cache_save`, `-cache-file`, `-cache-save`); no changes to `EMB`, `EMB.MULTI`, or the Ruby client.
- **Systems:** server only. Snapshot files are portable across restarts of the same machine; cross-arch portability is inherent (little-endian float32 written with explicit LE, matching the wire format).
- **Sequencing:** independent of `redis-style-info`; restores, `EMB.SAVE`, and the periodic loop need nothing from it. The `redis-style-info` change's `CONFIG SET` can drive `cache_file`/`cache_save` at runtime because the save loop re-reads runtime config each tick (no coupling either way).