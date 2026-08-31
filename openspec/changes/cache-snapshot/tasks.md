## 1. Cache iteration + format

- [ ] 1.1 Add `Cache.Visit(fn func(model, text string, value []byte) bool)` walking LRU front→back under the mutex; verify a unit test visits entries in recency order and can stop early
- [ ] 1.2 Implement `snapshot.go` writer: magic `EMBCACHE`, version uint32 LE = 1, count, then `(model, text, dim, vec)` with uint32 LE length prefixes, streaming to an `io.Writer`; verify a round-trip test (write → read) reproduces all entries byte-identically
- [ ] 1.3 Implement reader with strict length checks and version guard (unknown/tool-newer version → `ErrSnapshotVersion`); verify a test asserts truncated/corrupt files yield a valid-prefix + error (not a panic)

## 2. EMB.SAVE command

- [ ] 2.1 Add `-cache-file` CLI flag + `cache_file` YAML key (`internal/config/config.go`, parsed like `cache`); verify config test parses both and empty means disabled
- [ ] 2.2 Implement `EMB.SAVE` handler: atomic temp+fsync+rename dump when cache enabled and path configured; error reply when cache disabled or path unset; verify RESP-level test exercises both paths
- [ ] 2.3 Reply SHALL be a bulk string with total bytes + per-model counts; verify the parsing test asserts the reply contents
- [ ] 2.4 Add `EMB.SAVE` to `EMB.HELP`; verify help test includes it

## 3. Shutdown save + boot restore

- [ ] 3.1 Hook the snapshot dump into graceful shutdown (`server.Shutdown`, signaled in `cmd/emb/main.go`): dump when cache non-empty and path configured; verify an integration test sends SIGTERM to a warmed server and asserts a valid file on disk
- [ ] 3.2 Implement boot restore in `Server` bootstrap after model registration: only models already loaded receive entries; others skipped + counted (`cache_restore_skipped`); respect `maxBytes` (stop loading at budget); verify startup-order test: preloaded model's entries are hits on first request, lazy-model rows are skipped and counted, oversized snapshot stops at budget
- [ ] 3.3 Corrupt/version-mismatch file at boot: warn + boot empty (never crash); verify test boots and serves `EMB` normally with a garbage file present

## 4. Periodic saves (RDB-style)

- [ ] 4.1 Add `cache_save` YAML key + `-cache-save` flag and a pair parser (whitespace-separated `seconds changes`, even count, both > 0; malformed → startup error); verify config/parse tests cover `"900 1 300 10"`, `"2 5"`, empty, and invalid inputs (`"900"`, `"0 1"`, `"abc x"`)
- [ ] 4.2 Add `Cache.Snapshot() []SnapshotEntry` — reference-copy under the mutex, writer runs outside the lock (values immutable-on-replace, so refs stay valid after unlock); verify a test asserts entries remain correct when evicted/replaced mid-write, and a dirty counter on `Set` tracks changes since last save
- [ ] 4.3 Implement the periodic save loop goroutine: 1s ticker, trigger when any pair's seconds elapsed AND dirty changes ≥ threshold, single-flight (skip tick while a save runs), reset dirty count on success, log-and-continue on failure, stop on shutdown with a final save; verify trigger tests with real short pairs (`save 1 2` → saves after ≥1s and ≥2 changes) and a non-fatal failure test (unwritable path → server keeps serving, next tick retries)

## 5. Metrics + docs

- [ ] 5.1 Add `cache_restored_entries` and `cache_restore_skipped` to `EMB.INFO <model>` and the `INFO` `# Cache` section (when the INFO change exists, else `EMB.INFO` only); verify counters appear after a restore boot
- [ ] 5.2 Document `cache_file` + `cache_save` in `README.md` (Configuration + a "Cache snapshots" note: preload-gated warmth, `EMB.SAVE`, shutdown/periodic saves) and the `config.yaml` comment; verify rendered text matches behavior

## 6. Validation

- [ ] 6.1 Full suite green: `go test ./internal/server/` + Ruby client suite (`just all`) pass; pre-existing `TestAsyncTokenizerOverlapsWork` flake noted separately if it fails
- [ ] 6.2 End-to-end smoke: warm a server (≥ a few thousand distinct texts), `EMB.SAVE`, restart with `cache_file` set, verify a previously-seen text is a cache hit on first request (`EMB.INFO minilm` counters) and `cache_restored_entries` matches the snapshot count
- [ ] 6.3 Cross-restart determinism: dump, boot-restore, dump again; verify the second file is byte-identical in structure (entry count and vector payloads match; order may differ only by access recency)