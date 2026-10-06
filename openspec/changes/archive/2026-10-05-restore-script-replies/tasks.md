# Tasks

## 1. Classify entries by key family on restore

- [x] 1.1 Add a key-family helper beside `modelOf` in `internal/server/cache.go`
  that reports whether a cache key addresses a float32 embedding (`txt:`/`img:`).
  Verify with a table test covering a text key, an image key, and a script key.
- [x] 1.2 Use it in both branches of `readSnapshot` in
  `internal/server/snapshot.go` so the `Dim*4` length check applies only to
  embedding keys; opaque entries are gated by fingerprint alone. Verify with a
  test that a script reply for a `Dim = -1` model is retained while a `txt:`
  value of the wrong length is still skipped.
- [x] 1.3 Read the header's per-model dimension as a signed value so a `-1`
  dimension compares equal to the configured model instead of `4294967295`;
  non-negative dimensions stay byte-identical. Verify with a test that a
  `Dim = -1` model's entry is admitted/quarantined (not skipped) and that a
  positive dimension still matches.

## 2. Admit quarantined entries from the script path

- [x] 2.1 Call `s.admitQuarantine(model, entry)` in `evalScripted`
  (`internal/server/script.go`) immediately after the model resolves and before
  the cache lookup. Verify with a test that a quarantined reply for a model whose
  embedding pool never loads is served on the first `EMB.EVSHA` without running
  the script (the stored value differs from what the script would return).
- [x] 2.2 Ensure the admission is a no-op when persistence is off. Verify with a
  test that a server with no snapshot coordinator and no quarantine serves the
  script normally.

## 3. Prove the round-trip end to end

- [x] 3.1 Add a save/restore test: build a cache holding a script reply and an
  embedding for a model, write the snapshot with the model's real fingerprint,
  read it back with the model unloaded, admit, and assert both entries are
  present and byte-identical. Verify with `go test ./internal/server/ -run
  Snapshot -count=1`.
- [x] 3.2 Run the full snapshot and script suites and the linter:
  `nix develop --command bash -c 'go test ./internal/server/ ./internal/config/
  -count=1 && golangci-lint run ./internal/server/'`.
- [x] 3.3 On the deployed sandbox, confirm the fix: seed a scripted reply, let a
  graceful restart save a snapshot, and check `cache_restore_entries > 0` for
  `laya-real` with a first script call served as a cache hit.

## Workflow follow-up

- `just sandbox-deploy` by hand after the change lands.
- Archive the change and sync the `cache-snapshots` delta once the deployed
  restart check is recorded.
