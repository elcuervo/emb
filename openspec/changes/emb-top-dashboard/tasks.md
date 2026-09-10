## 1. Dependencies

- [x] 1.1 Add `github.com/NimbleMarkets/ntcharts` (pinned v1 tag), `charmbracelet/bubbletea` v1, `charmbracelet/lipgloss` v1 to go.mod and verify `go mod tidy` is clean and `go build ./cmd/emb-top` compiles
- [x] 1.2 Confirm `go vet ./...` and `go test ./...` still pass with the new deps present

## 2. RESP2 client (internal/embtop/resp.go)

- [x] 2.1 Implement minimal RESP2 reader/writer (bulk strings, integers, arrays, errors) and verify unit tests cover decode of the reply shapes EMB.MODELS / EMB.INFO / EMB.STATS produce
- [x] 2.2 Implement dialing (plain TCP and TLS) plus `AUTH` and verify a test connects to a real server, authenticates, and reads EMB.STATS
- [x] 2.3 Implement one pipelined poll (EMB.MODELS → EMB.INFO per model → EMB.STATS, single write/read) and verify a unit test asserts commands are concatenated into one write and replies parsed in order

## 3. Sampler and rates (internal/embtop/stats.go)

- [x] 3.1 Implement cumulative-counter diffing (req/s, tok/s, err/s, per-model rates, CPU% from cpu_user/sys_usec, cache hit ratio) with a ring buffer of N samples and verify deterministic unit tests (fixed counter feeds, no sleeps) assert exact rates and window eviction
- [x] 3.2 Implement counter rebasing on reconnect (discard previous snapshot, clamp negative deltas to 0) and verify unit tests cover reconnect and counter-reset cases

## 4. Headless mode (internal/embtop/once.go, flags)

- [x] 4.1 Implement `-once -samples N -interval D [-addr] [-password] [-tls]` printing one machine-readable line per poll and verifying against a running server that output contains aggregate + per-model fields and exits 0
- [x] 4.2 Verify unreachable node at startup in `-once` mode exits non-zero with a clear error (integration test + manual check)

## 5. TUI skeleton (cmd/emb-top)

- [x] 5.1 Implement bubbletea app with a 1s ticker, lipgloss-styled header (node addr, version, uptime, models, poll interval) and status line, and verify `q` quits and `p`/space pauses polling (manual + snapshot)
- [x] 5.2 Implement layout with resize handling (header / charts / per-model panel / gauge row / footer) and verify the app renders without panic at several terminal sizes

## 6. Charts and eye candy

- [x] 6.1 Render aggregate req/s and tok/s as autoscaling streamline charts with distinct per-series colors and verify values track sampler output in a headless-render test
- [x] 6.2 Render per-model panel: braille sparkline per model plus req/s, tok/s, avg latency, errors, and metadata (dim, pooling, quantization, batching); verify j/k scrolling with many models
- [x] 6.3 Render gauge row (cache hit ratio, RSS mem, CPU %, connections, active requests) as barcharts/sparklines and verify values update per poll
- [x] 6.4 Add error emphasis (red flash on rising error rates), connection-lost banner with last-good time and auto-reconnect, and `r` window reset; verify by killing/restarting a server under the TUI
- [x] 6.5 Add `?` help overlay and verify it lists all keybindings

## 7. Distribution: build + Docker

- [x] 7.1 Update `just build` to also produce `bin/emb-top` (CGO_ENABLED=0, version ldflags) and verify both binaries build
- [x] 7.2 Update Dockerfile builder stage to build `emb-top` statically and runtime stage to `COPY` it to `/usr/local/bin/emb-top`; verify a docker build on linux/amd64 yields an image where `/usr/local/bin/emb-top` runs
- [x] 7.3 Verify the image's `emb-top` connects to the image's own `emb` node (container smoke test)

## 8. Distribution: emb-server gem

- [x] 8.1 Add `bin/emb-top` wrapper (platform switch to `lib/emb-server/emb-top-binary-<platform>`, no onnxruntime dependency) and verify it execs the binary with passed args
- [x] 8.2 Update `emb-server.gemspec` (`files` += `emb-top-binary-*`, `executables` += `emb-top`) and `just validate-gems` to copy `bin/emb-top` into the gem dir before building; verify `gem build` + `gem install` yields `emb-top` on PATH that connects to a node without the onnxruntime gem

## 9. CI / release workflow

- [x] 9.1 Extend the release workflow to build `emb-top` per supported platform, include it in gem builds and the Docker image, and verify a release run produces the new artifacts

## 10. Docs, lint, integration

- [x] 10.1 Add README section on `emb-top` (usage, flags, keys, terminal requirements, pairing with `redis-benchmark`/`just bench-*`) and verify it reads coherently
- [x] 10.2 Add integration test: start server, run `emb-top -once -samples 3`, assert line format and non-negativity; wire into `just test`
- [x] 10.3 Run `just format`, `just lint`, `just test`, and `just all` (server + Ruby suite + validate-gems) and verify everything passes end-to-end
## 11. Server: MONITOR

- [x] 11.1 Implement `internal/server/monitor.go` (seq-numbered bounded ring, `Add`/`Since(after, limit)`/`LastSeq`) and verify unit tests cover ring wrap, ordering, and limit clamping
- [x] 11.2 Wire `MONITOR` into the mux + help text and record events in `handleEMB`/`handleEMBMULTI` completion paths (latency around inference, error flag, cache hits, no text payloads); verify handler tests against an in-process server (idle → empty, 1 request → 1 event, incremental fetch, error events, ≤8192 bound)

## 12. Client: monitor parsing + percentile window

- [x] 12.1 Extend `embtop.Client.Poll` to pipeline `MONITOR <afterSeq> [limit]` and parse event replies; verify fake-server tests assert the pipeline shape and event decoding
- [x] 12.2 Add sampler event ingestion (bounded event window) + p50/p95/p99 computation for aggregate and per model; verify deterministic unit tests with synthetic latencies
- [x] 12.3 Handle ring-buffer gaps (rebased, no error) and verify unit tests; extend `-once` output with `lat_p50_us`/`lat_p95_us`/`lat_p99_us` and verify the scripted-server test

## 13. Dashboard v2

- [x] 13.1 Add model-activity heatmap panel (rows = models, cols = polls, color = req/s); verify headless render test shows traffic on busy rows
- [x] 13.2 Add p95 latency stream chart + per-model p50/p95/p99 in model rows + event ticker line; verify headless render test
- [x] 13.3 Restyle (theme, spacing, sorted model panel) and verify render tests + no-panic at several sizes

## 14. just all / nix develop

- [x] 14.1 Switch `test-two-models.yaml` `minilm` to `model_repo: Xenova/all-MiniLM-L6-v2` and verify `just all` passes end-to-end (server starts with downloads, Ruby suite green)
- [x] 14.2 Verify `nix develop --command 'just build && just lint && just test && just validate-gems'` is clean and `go.mod`/`go.sum` are tidy
