## 1. Dependency

- [x] 1.1 Add `github.com/docker/go-units` to `go.mod`

## 2. LRU cache implementation

- [x] 2.1 Create `internal/server/cache.go` with `Cache` struct (mutex, list, map, counters)
- [x] 2.2 Implement `NewCache(maxBytes int)` constructor
- [x] 2.3 Implement `Get(key string) ([]byte, bool)` — hits/misses via atomic counters
- [x] 2.4 Implement `Set(key string, value []byte)` — insert, evict LRU if over budget
- [x] 2.5 Implement `Stats()` returning `CacheStats` (hits, misses, evictions, entries, maxEntries, memoryBytes)
- [x] 2.6 Implement `autoTuneCache(dim int) int` in `cache.go` using `TotalSystemMemory()`
- [x] 2.7 Implement `parseCacheConfig(s string) (int64, error)` — handles `"auto"`, size strings via `docker/go-units`, empty string

## 3. Config changes

- [x] 3.1 Add `Cache string` field to `Config` struct in `internal/config/config.go`
- [x] 3.2 Add `-cache <val>` flag handling to `ParseFlags` in `internal/config/config.go`

## 4. Server wiring

- [x] 4.1 Add `cache *Cache` field to `Server` struct in `internal/server/server.go`
- [x] 4.2 Update `New` to accept cache config string and create cache if non-empty
- [x] 4.3 Inline cache check in `handleEMB` — no separate helper needed, logic is per-handler
- [x] 4.4 Update `handleEMB` to check cache before model load, handle partial hits
- [x] 4.5 Update `handleEMBMULTI` to check cache before model load
- [x] 4.6 Update `handleINFO` to include cache stats in response
- [x] 4.7 Update `handleSTATS` to include aggregate cache stats
- [x] 4.8 Update `handleHELP` to document cache behavior

## 5. Main wiring

- [x] 5.1 Pass cache config to `server.New` in `cmd/emb/main.go`
- [x] 5.2 Cache is dim-agnostic — stores raw `[]byte`, no dim needed

## 6. Tests

- [x] 6.1 `TestCacheGetSet` — basic get/set, hit returns correct bytes
- [x] 6.2 `TestCacheEviction` — insert beyond budget, verify LRU evicted
- [x] 6.3 `TestCacheHitCounts` — hits/misses/evictions increment correctly
- [x] 6.4 `TestCachePartialHit` — partial text hit with server handler
- [x] 6.5 `TestCacheDisabled` — no cache config, normal operation
- [x] 6.6 `TestAutoTuneCache` — covered by `TestParseCacheConfig` auto case
- [x] 6.7 `TestParseCacheConfig` — "auto", "512MB", "", invalid
- [x] 6.8 `TestCacheOnINFO` — cache stats visible in EMB.INFO output

## 7. Config example

- [x] 7.1 Add commented-out `cache` field to `config.yaml`

## 8. Go code quality tooling

- [x] 8.1 Expand `.golangci.yml` with additional linters: `errcheck`, `gosimple`, `ineffassign`, `unused`, `stylecheck`, `gosec`, `revive`, `gocritic`
- [x] 8.2 Run `golangci-lint fmt` across all Go packages to apply formatting
- [x] 8.3 Run `golangci-lint run --fix` to auto-fix lint issues
- [x] 8.4 Manually fix remaining lint issues that cannot be auto-fixed
- [x] 8.5 Verify `golangci-lint run ./...` passes with zero issues
- [x] 8.6 Add `lint` step to `.github/workflows/ci.yml` running `golangci-lint run ./...`
- [x] 8.7 Run `just format` and `just lint` to confirm both pass
