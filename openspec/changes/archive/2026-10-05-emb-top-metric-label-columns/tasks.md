# Tasks

## 1. Fixed label columns

- [x] 1.1 Add `fixedLabeledCol` (label first, value padded into the rest of the slot)
- [x] 1.2 Use it for `p50`/`p95`/`avg` and `err` in `modelRow`

## 2. Checks

- [x] 2.1 Test that an idle row's `avg` and `err` share columns with a busy row's `p50` and `err`
- [x] 2.2 Run `go test ./cmd/emb-top/` and `golangci-lint run ./cmd/emb-top/` inside `nix develop`

## 3. Recording

- [x] 3.1 Re-record the landing-page plate and docs capture (`just website-topviz`)
