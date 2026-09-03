## 1. Memory sampling primitives

- [x] 1.1 Add `CurrentMemoryUsage() (rss bytes, fromRSS bool)` and `TotalSystemMemory()` backed by gopsutil (`process.MemoryInfo().RSS`, `mem.VirtualMemory().Total`; heap fallback for unsupported platforms) in `internal/registry/resources.go`, replacing the per-OS `sysmem_*.go` files; verify with a unit test / quick `go test ./internal/registry/` that the value is positive and grows after allocation
- [x] 1.2 Confirm the package builds with `GOOS=darwin go build ./internal/registry/` and `GOOS=linux go build ./internal/registry/` (no build-tagged files remain — gopsutil handles all platforms, including real RSS on macOS)
- [x] 1.3 Add CPU (`process.Times()` user/system usec — kernel-accounted from boot, no runtime/metrics NaN window) and keep the non-blocking Go-heap sampler + `NumGoroutine()`; verify unit tests cover monotonicity and heap growth

## 2. INFO sections

- [x] 2.1 Extend `infoSnapshot` and `infoSnapshot()` (`internal/server/info.go`) with memory/CPU/bytes fields gathered from the new primitives + `TotalSystemMemory()` + new net-byte counters
- [x] 2.2 Add `# Memory` and `# CPU` render blocks in `buildInfoSections()` after `# Stats` and add `memory`/`cpu` to the section selection map; verify `info_config_test.go` scenarios for `INFO memory`, `INFO cpu`, and `INFO memory cpu` pass (only-requested-sections, union, empty for bogus)
- [x] 2.3 Add `total_net_input_bytes`/`total_net_output_bytes` to the `# Stats` block and verify existing `TestServerRedisINFO`/section-count tests pass

## 3. Network byte counters

- [x] 3.1 Add `countingConn` (embeds `redcon.Conn`, overrides `Write*` methods to accumulate returned byte counts) and a mux wrapper adding `len(cmd.Raw)` to the input counter, wired in `NewServer` with `resourceHandler{mux}` (`internal/server/server.go`)
- [x] 3.2 Add a byte-counter integration test: open a socket, send a known-size `EMB`/`PING` command, read reply, assert `total_net_input_bytes`/`total_net_output_bytes` increased by at least the wire sizes (`INFO stats` before/after), and assert counters never decrease across polls

- [x] 3.3 Verify `go vet ./...` and `golangci-lint run ./...` stay clean after the wrapper changes

## 4. EMB.STATS resource fields

- [x] 4.1 Replace the dead `mem` value in `handleSTATS` (`internal/server/server.go`) with process RSS in MB (heap fallback) from the snapshot data, and add `cpu_user_usec`, `cpu_sys_usec`, `goroutines` pairs; update `conn.WriteArray(32)` to the new count
- [x] 4.2 Verify all `EMB.STATS` tests pass including `server-stats-observability` count-parity scenarios and that `mem` is positive after a loaded model

## 5. Leak regression test

- [x] 5.1 Add a white-box leak test (gated fakes, no sleeps, per the batcher-budget test pattern) that runs N batches through the pipeline and asserts `NumGoroutine()` returns to baseline and heap in-use stays flat after `runtime.GC()` passes; verify it passes `go test ./internal/pipeline/ ./internal/server/ -count=1` repeatedly
- [x] 5.2 Update `EMB.HELP` output to mention `INFO memory`/`INFO cpu` sections and verify the `EMB.HELP` test expectations

## 6. Validation

- [x] 6.1 Run `just test`, `just lint`, `just format` and confirm they pass inside `nix develop`
- [x] 6.2 Smoke-test live: run `./bin/emb -config test-two-models.yaml -cache auto -listen :16379`, then `redis-cli -p 16379 INFO memory` and `INFO cpu` and `EMB.STATS` and confirm sane non-zero values and correct ordering