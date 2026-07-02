## Why

Every `EMB` request runs tokenization + ONNX inference + pooling, which takes ~3-5ms even for short texts. In many workloads texts repeat across requests (popular queries, known documents, retries). Without a cache, every repeated text pays the full inference cost.

Adding an LRU cache means repeated texts return in <1µs — the cache lookup is 3-4 orders of magnitude faster than running inference again.

## What Changes

- Add a global LRU cache keyed by `(model, text)` returning the raw `[]byte` embedding
- Configurable via a new `cache` field in YAML (`"auto"`, `"1GB"`, `"256MB"`, or empty string for disabled)
- Same via the `-cache` CLI flag
- When `cache: "auto"`, the cache size auto-tunes based on system memory (reuses existing `totalSystemMemory()` plumbing)
- Cache stats exposed in `EMB.INFO` and `EMB.STATS` (hits, misses, evictions, entries, memory)
- Cache hits skip model loading entirely — a hit doesn't call `GetOrInit`
- Expand `golangci-lint` configuration with additional linters (errcheck, gosimple, ineffassign, unused, stylecheck, gosec, revive, gocritic) — the Go equivalent of RuboCop for Ruby
- Wire `golangci-lint` into CI as a dedicated lint step
- Run `golangci-lint fmt` and `golangci-lint run --fix` across all Go packages to address all issues

## Capabilities

### New Capabilities
- `lru-cache`: Global LRU embedding cache with auto-tune or explicit memory budget
- `go-quality-tooling`: Enhanced golangci-lint configuration with expanded linters, CI integration

### Modified Capabilities
- (none)

## Impact

- `go.mod` — new dependency: `github.com/docker/go-units` for human-readable size parsing
- `internal/config/config.go` — new `Cache string` field on `Config`, new `-cache` flag in `ParseFlags`
- `internal/server/server.go` — new `cache` field on `Server`, cache check in `handleEMB` and `handleEMBMULTI`, cache stats in `handleINFO` and `handleSTATS`
- `internal/server/cache.go` — new file: LRU cache implementation (`container/list` + `map`)
- `internal/server/server_test.go` — new tests for cache hit/miss/eviction/metrics
- `config.yaml` — optional `cache` field (commented out by default)
- `.golangci.yml` — expanded linter set: errcheck, gosimple, ineffassign, unused, stylecheck, gosec, revive, gocritic
- `.github/workflows/ci.yml` — new `lint` step running `golangci-lint run ./...`
- Various `.go` files — formatting and lint fixes from `golangci-lint fmt` and `golangci-lint run --fix`
