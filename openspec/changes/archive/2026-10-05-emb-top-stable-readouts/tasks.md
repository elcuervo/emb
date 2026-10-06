# Tasks

## 1. Fixed-width readouts

- [x] 1.1 Pad the health banner chips and the status label
- [x] 1.2 Pad the header's uptime and model count
- [x] 1.3 Give the gauge captions a fixed value column that shrinks with the band
- [x] 1.4 Pad the connection line and the event ticker

## 2. Checks

- [x] 2.1 Test that the banner chips keep their width as values grow
- [x] 2.2 Test that a gauge value keeps its column
- [x] 2.3 Run `go test ./cmd/emb-top/` and `golangci-lint run ./cmd/emb-top/` inside `nix develop`

## 3. Recording

- [x] 3.1 Re-record the landing-page plate and docs capture (`just website-topviz`)
