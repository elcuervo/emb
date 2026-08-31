## Purpose

Versioned binary snapshots of the embedding cache — dump via `EMB.SAVE` or on graceful shutdown, and restore into memory at boot so already-processed texts are instantly hot for a given model.

## ADDED Requirements

### Requirement: Snapshot file format

A snapshot SHALL be a single binary file: magic `EMBCACHE`, a format version (uint32 LE), an entry count, then entries of `(model name, text, dim, vector bytes)` with little-endian uint32 length prefixes. Vector bytes SHALL be the model's raw float32 embedding payload exactly as served over RESP2 (le format, `dim × 4` bytes, no widening).

#### Scenario: Round-trip preserves vectors

- **WHEN** a cache with model `minilm`, text `"hello world"`, and a 384-dim vector is dumped and then loaded
- **THEN** the restored entry SHALL return byte-identical output for `EMB minilm "hello world"`

#### Scenario: Unknown format version

- **WHEN** a snapshot file carries a format version newer than the binary supports
- **THEN** the server SHALL skip loading it with a warning and continue booting with an empty cache

### Requirement: EMB.SAVE command

The server SHALL respond to `EMB.SAVE` with a synchronous dump of the cache to the configured `cache_file` path using an atomic write (temp file + rename). The reply SHALL be a bulk string containing bytes written and per-model entry counts.

#### Scenario: Dump writes atomically

- **WHEN** `EMB.SAVE` is issued with a configured path
- **THEN** the reply SHALL report total bytes and per-model counts
- **THEN** the target path SHALL contain a complete snapshot (no partial file visible, even mid-write)

#### Scenario: No cache or no path

- **WHEN** `EMB.SAVE` is issued but the cache is disabled or `cache_file` is unset
- **THEN** the server SHALL reply with an error explaining no snapshot target is configured

### Requirement: Save on graceful shutdown

When a `cache_file` is configured, the cache is enabled, and the cache is non-empty, a graceful shutdown (SIGINT/SIGTERM) SHALL write a snapshot before the process exits.

#### Scenario: Shutdown produces a snapshot

- **WHEN** the server receives SIGTERM after serving cached texts with `cache_file` configured
- **THEN** the configured path SHALL contain a valid snapshot after the process exits

### Requirement: Restore on boot

When a `cache_file` is configured, the cache is enabled, and the file exists, the server SHALL load entries into the cache after model registration. Rows whose model is not loaded at restore time SHALL be skipped and counted. The restored cache SHALL respect the configured `maxBytes` budget.

#### Scenario: Restored entries are hot

- **WHEN** the server boots with a snapshot containing `(minilm, "hello world")` and `minilm` is preloaded
- **THEN** the first `EMB minilm "hello world"` SHALL be a cache hit without inference (verified via `EMB.INFO minilm` counters)

#### Scenario: Unloaded model rows are skipped

- **WHEN** the snapshot contains entries for a model that is not loaded at restore time
- **THEN** those entries SHALL be skipped, counted in `cache_restore_skipped`, and the server SHALL boot normally

#### Scenario: Restore respects the budget

- **WHEN** the snapshot's total size exceeds the configured `cache` budget
- **THEN** loading SHALL stop once the budget is reached (or evict the LRU tail) and the server SHALL remain functional

#### Scenario: Corrupt file does not crash boot

- **WHEN** the snapshot file is truncated or corrupted
- **THEN** the server SHALL log a warning, load nothing (or the valid prefix), and continue booting

### Requirement: Periodic snapshots

When `cache_save` is configured, the server SHALL periodically dump the cache in the background. `cache_save` SHALL accept zero or more Redis-style `seconds changes` pairs (whitespace-separated, even count, both parts positive integers). A background save SHALL trigger when any pair's seconds have elapsed since the last save AND its changes threshold has been exceeded since the last save. Periodic saves SHALL NOT block request handling. A successful save SHALL reset the change counter; save errors SHALL be logged and never abort the server.

#### Scenario: Pairs trigger by time and change count

- **WHEN** `cache_save` is `"2 5"` and 3 new entries are cached, then 4 more (7 ≥ 5) 2+ seconds after the last save
- **THEN** a background save SHALL occur and the file SHALL contain the 7 entries

#### Scenario: Only time elapsed, not enough changes

- **WHEN** `cache_save` is `"2 5"`, 10 seconds pass, and only 2 new entries were cached
- **THEN** no save SHALL occur (the required change count was not reached)

#### Scenario: Only changes, not enough time

- **WHEN** `cache_save` is `"2 5"` and 50 new entries are cached within the first second after a save
- **THEN** no save SHALL occur until 2 seconds have elapsed

#### Scenario: No pairs disables periodic saves

- **WHEN** `cache_save` is empty or unset
- **THEN** snapshots SHALL occur only via `EMB.SAVE` and graceful shutdown

#### Scenario: Save failure is non-fatal

- **WHEN** a periodic save fails (e.g., unwritable path)
- **THEN** the server SHALL log the error and continue serving; the next qualifying tick SHALL retry

#### Scenario: Final save on shutdown

- **WHEN** the server is shutting down gracefully while periodic saves are enabled
- **THEN** a final snapshot SHALL be written regardless of whether the interval had elapsed

### Requirement: Restore metrics

`EMB.INFO <model>` SHALL report `cache_restored_entries` and `cache_restore_skipped` for the model, alongside the existing cache fields.

#### Scenario: Counters appear after restore

- **WHEN** a model had restored entries and skipped rows at boot
- **THEN** `EMB.INFO <model>` SHALL include `cache_restored_entries` equal to the loaded count and `cache_restore_skipped` equal to the skipped count