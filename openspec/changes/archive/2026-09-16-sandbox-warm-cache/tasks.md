## 1. Persist the sandbox cache on the volume

- [x] 1.1 Add `cache_file: /data/cache.embcache` and `cache_save: 30m` to `website/repl/sandbox.yaml`, and replace the "No cache_file: this is a sandbox" sentence with the volume-backed rationale; verify the shipped file carries both keys and no longer says a restart starts cold (`grep -n 'cache_file\|cache_save\|No cache_file' website/repl/sandbox.yaml`)
  - Both keys are present (`cache_file: /data/cache.embcache`, `cache_save: 30m`) and the old sentence is gone. The shipped config loads through `internal/config.Load` with `cache_file="/data/cache.embcache" cache_save="30m"`, and the locally generated config (both keys cleared) still loads with the empty pair — which matters because `validatePersistence` rejects `cache_save` without `cache_file`.

## 2. Give the flush time to finish on Fly

- [x] 2.1 Add `kill_timeout = 40` to `website/repl/fly.toml` with the rationale that Fly's 5s default would SIGKILL the flush; verify the TOML parses and carries the key (`fly config validate -c website/repl/fly.toml` where flyctl is installed, otherwise a TOML parse)
  - `tomllib` parses the file and reports `kill_timeout=40` with no `kill_signal` (the default SIGINT, which `run.sh` and `emb` both trap). `fly config validate` was not run: flyctl is not installed here.

## 3. Keep the refusal truthful

- [x] 3.1 Change the `EMB.SAVE` reason in `website/repl/allowlist.go` from "the sandbox persists nothing" to a statement about visitor-triggered writes; verify `just sandbox-test` passes
  - The reason is now `state writes are not visitor-triggerable`; `CGO_ENABLED=0 go test ./website/repl/` passes and `gofmt -l website/repl` is clean.
- [x] 3.2 Add a focused test in `website/repl/bridge_test.go` asserting the `EMB.SAVE` refusal does not claim the sandbox is stateless; verify the new test passes under `just sandbox-test`
  - `TestEmbSaveRefusalNamesTheBoundaryNotStatelessness` asserts the refusal is produced, does not contain "persists nothing", and names the boundary.

## 4. Deploy builds first; local runs stay cold

- [x] 4.1 Make `sandbox-deploy` depend on `sandbox-build`; verify `just --dry-run sandbox-deploy` runs the build before `fly deploy`
  - `just --dry-run sandbox-deploy` prints the `sandbox-build` steps (`go build ./cmd/emb`, `./cmd/emb-top`, `./website/repl`) before `fly deploy . -c website/repl/fly.toml`.
- [x] 4.2 Clear the snapshot path in the `website-dev` and `website-demos` generated configs; verify `just --dry-run website-dev` and `just --dry-run website-demos` carry the expression and a hand-run of the same `sed` yields `cache_file: ""`
  - Both recipes clear `cache_file` and `cache_save` (the pair, because `cache_save` alone fails validation), and the hand-run `sed` yields `cache_file: ""` with `cache_save: ""`.

## 5. Documentation

- [x] 5.1 Note the volume-backed warm cache and the raised kill window in `website/README.md`; verify the note is present and introduces no hosted-offering claim
  - The Deploying section gains a paragraph naming `cache_file`, the interval, the restore-before-ready order, the raised kill window, and the file's disposability. It restates no offering, tier, or SLA.

## 6. Deployed verification (needs an authenticated flyctl)

- [x] 6.1 Run `just sandbox-deploy` and verify it builds first and the deploy succeeds; the first boot after this change is cold, as before
  - `just sandbox-deploy` ran the `sandbox-build` steps (`./cmd/emb`, `./cmd/emb-top`, `./website/repl`) before `fly deploy`. Image `deployment-01M2NMFW8CV831C489G1JFMVQH` rolled out to machine v8; `fly status` reports `started`, `2 total, 2 passing`. The first boot after the deploy answered `INFO cache` with `cache_snapshot_file:/data/cache.embcache`, `cache_snapshot_enabled:true`, `cache_restore_entries:0`, empty `cache_restore_error` — cold, as before.
- [x] 6.2 Warm the cache, run `fly machine restart -a emb-sandbox`, and verify the next boot restores entries (the restore counters from `INFO cache` on the bridge are non-zero)
  - Warmed with `EMB minilm hello world` twice (`cache_entries:2`, `cache_hits:2`, repeat `elapsed_us` 248 vs 9504). `fly machine restart 784567db5de028` wrote `/data/cache.embcache` (3717 B, mode `0600`) on shutdown. The next boot restored it: `cache_restore_quarantined:2` until minilm lazy-loaded, then the first request answered `cache_hits:2 cache_misses:0` with no re-inference, and a follow-up cost 386 µs. The deployed machine carries `stop_config.timeout = 40s`, so the raised window is in effect.
- [x] 6.3 Remove or corrupt the snapshot and verify the next boot starts cold and still reports ready, rather than failing to start
  - Removed `/data/cache.embcache` during a clean boot and restarted, so the clean cache skipped the shutdown save. The next boot answered `cache_restore_entries:0`, `cache_restore_quarantined:0`, empty `cache_restore_error`, `/api/ready` `ready:true`. It then warmed again normally. The deployment is left with a warm snapshot on the volume.
