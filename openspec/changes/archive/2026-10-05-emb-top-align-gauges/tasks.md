# Tasks

## 1. Shared row grid

- [x] 1.1 Add a `rowLayout` derived from the terminal width: identity width, the metric slots that fit, and the strip width
- [x] 1.2 Make `modelRow` render its segments on that layout, reserving the p50 and p95 slots and filling the first with `avg` when no events exist
- [x] 1.3 Resize the three gauge bars to the layout's band widths and place them on the same grid

## 2. Checks

- [x] 2.1 Add a test asserting the gauge bands start on the model row's zones at several widths
- [x] 2.2 Run `go test ./cmd/emb-top/` and `golangci-lint run ./cmd/emb-top/` inside `nix develop`

## 3. Recording

- [x] 3.1 Re-record the landing-page plate and docs capture (`just website-topviz`) and update the embedded frame
