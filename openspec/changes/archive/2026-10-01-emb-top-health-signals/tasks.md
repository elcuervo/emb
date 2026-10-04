# Tasks

## 1. Stable model list

- [x] 1.1 Remove the per-render req/s `sort.SliceStable` in `modelsView()` and `hotRows()` so both render `m.modelOrder` (server `EMB.MODELS` order); verify with a new `cmd/emb-top` test that feeds two polls with swapped/fluctuating req/s and asserts row order is unchanged
- [x] 1.2 Make per-model metric columns fixed-width (pad each segment to a constant width, compute the fit budget from those widths) so `9.9` → `10.0` cannot shift trailing columns; verify in `cmd/emb-top/width_test.go` that rendered row widths/positions are stable across value changes
- [x] 1.3 Update `cmd/emb-top/main_test.go` expectations for the new stable order and confirm `go test ./cmd/emb-top/` passes

## 2. Health status and banner

- [x] 2.1 Add `cmd/emb-top/health.go` with a pure `health(...) (status, reasons)` over connection state, poll count, recent error ratio, p95-vs-baseline, CPU, and cache hit rate, using the fixed default thresholds from design.md; verify with a table-driven test covering no-data, healthy, degraded, critical, and disconnected
- [x] 2.2 Render the health banner in `View()` with reason chips, using reserved semantic colors distinct from `modelColors`; verify with a render test asserting the banner text/status for given snapshots
- [x] 2.3 Account for the new banner line in the per-model reservation (`m.height - N`) and confirm the full dashboard still renders within the fixed frame; verify via the existing width/layout tests

## 3. Per-model health indicators

- [x] 3.1 Add a `modelHealth(...)` helper returning a status dot, rising/falling arrows for error rate and latency, and an explicit `idle` state when a model saw no traffic in the window; verify with a table-driven test for busy-ok, erroring, idle, and rising-latency cases
- [x] 3.2 Wire the indicator into `modelsView()` rows; verify with a render test that a model with errors is visually emphasized and an idle model shows the idle state

## 4. Latency percentile band

- [x] 4.1 Record per-poll `p50/p95/p99` (from the existing `Sampler.Latency()` call in `applyResult`) into a bounded `latHist` ring on the TUI model; verify with a test that the ring fills and evicts at the window bound
- [x] 4.2 Implement the band renderer as a pure function over `latHist` (shade cells between p50 and p99, draw p95 as a bright rune), following the hand-rolled heatmap approach; verify with a table test of empty, single-sample, all-equal, and wide-spread inputs
- [x] 4.3 Replace the single-series `latChart` with the band in `View()`/`layoutCharts()`/`reset()`; verify with a render test that the band advances per poll and stays within its allotted size

## 5. Integration and artifacts

- [x] 5.1 Verify `-frames` mode still emits complete 120×40 frames after the layout change (new banner + band) by running the frames test/`internal/embtop` e2e test and adjusting `frameHeight` or the reservation if needed
- [x] 5.2 Run `nix develop --command bash -c 'go test ./cmd/emb-top/ ./internal/embtop/ && golangci-lint run ./cmd/emb-top/... ./internal/embtop/...'` and `just verify-harness`; confirm all pass
- [x] 5.3 Re-record and publish the website plate with `just website-topviz` and confirm the published-tree check passes
- [x] 5.4 Update `helpView()` / footer copy to mention the health banner and band, and confirm the help screen documents the new signals