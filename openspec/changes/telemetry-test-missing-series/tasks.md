# Tasks

## 1. Wait for the end-to-end request delta

- [x] 1.1 Update `TestTelemetryEndToEnd` to treat exports without `emb.requests` as zero and keep waiting until an export contains a positive request delta or the existing deadline expires; preserve the positive-delta assertion and verify with `go test -race ./internal/server/ -run '^TestTelemetryEndToEnd$' -count=1` inside `nix develop`.
