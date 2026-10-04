# Tasks

## 1. Server: deterministic EMB.MODELS

- [x] 1.1 Sort `Registry.List()` by model name and verify with a registry test that repeated `List()` calls on the same registry return the same name-sorted order
- [x] 1.2 Confirm all `List()` callers stay correct (`Fingerprints`, `configModels`, `infoSnapshot`, `handleSTATS`) and that `go test ./internal/registry/ ./internal/server/` passes

## 2. emb-top: stable first-seen rows

- [x] 2.1 Replace the per-poll rebuild of `modelOrder`/`known` in `applyResult` with first-seen reconciliation (drop vanished, append new) and verify a test that feeds the same models in a different order each poll and asserts rows never move
- [x] 2.2 Confirm model colors stay keyed to the stable order and `go test ./cmd/emb-top/` passes

## 3. emb-top: legible traffic and cleaner header

- [x] 3.1 Show each heatmap row's current req/s after its strip and update the legend to name the scale and time direction; verify in `hmcheck_test.go`
- [x] 3.2 Drop req/s, p95, and connection state from the header (the health line owns them); verify the header/health render tests still pass
- [x] 3.3 Confirm the dashboard still fits the terminal width via `width_test.go`

## 4. Integration and artifacts

- [x] 4.1 Run `go test ./...`, `golangci-lint run ./...`, `just verify-harness`, and `just deadcode`; confirm all pass
- [x] 4.2 Re-record and publish the website plate with `just website-topviz` and confirm the published-tree check passes; confirm the plate's rows are ordered and stable
