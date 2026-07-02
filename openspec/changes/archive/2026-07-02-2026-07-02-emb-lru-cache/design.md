## Context

The emb server processes `EMB <model> <text> [text...]` commands through a pipeline of tokenize → ONNX inference → pool/normalize → return bytes. ONNX inference is the dominant cost at ~3-5ms per request. There is currently no caching — every request runs the full pipeline.

The existing `autoTuneWorkers` function in `registry.go` uses `totalSystemMemory()` to estimate available memory and set worker count. The same `totalSystemMemory()` function is available for cache auto-tuning.

## Goals / Non-Goals

**Goals:**
- Cache embeddings so repeated texts return without inference
- Configurable via simple `cache` field: `"auto"`, `"1GB"`, `"256MB"`, or empty (off)
- Auto-tune cache size from system memory when `"auto"` is set
- Expose cache metrics in `EMB.INFO`
- Cache hits bypass model loading entirely

**Non-Goals:**
- Per-model cache config (global cache only)
- Disk persistence (in-memory only; disk cache can be a future change)
- Cache invalidation via command (add later if needed)
- Fuzzy matching / near-duplicate detection (exact text match only)

## Decisions

### 1. Global cache, not per-model

A single global cache avoids per-model wiring, keeps the flag simple (`-cache 1GB`), and lets memory be shared efficiently across all models. The cache key is `"model:text"` to avoid collisions between models.

### 2. Config as string: `"auto"`, `"1GB"`, `""`

The `cache` field is typed as `string` in the Go config struct. YAML naturally accepts quoted strings. The flag `-cache auto` or `-cache 1GB` passes a string argument. Three states:
- Empty string / `""` — cache disabled (default)
- `"auto"` — auto-tune max entries from system memory
- Any other string — parsed via `docker/go-units.FromHumanSize()` for bytes, then converted to entries based on embedding dimension

### 3. LRU implementation: `container/list` + `map`

Roll our own LRU instead of importing `hashicorp/golang-lru`. The implementation is ~100 lines:

```
type entry struct {
    key   string
    value []byte
}

type Cache struct {
    mu        sync.Mutex
    maxBytes  int64
    curBytes  int64
    ll        *list.List
    entries   map[string]*list.Element
    hits      atomic.Int64
    misses    atomic.Int64
    evictions atomic.Int64
}
```

The byte budget (`maxBytes`) is derived at creation time from the config string:
- `"auto"` → call `autoTuneCache(dim)` which uses `totalSystemMemory()` to estimate
- explicit size → `docker/go-units.FromHumanSize()` converts to bytes
- Calculating entries: `budget / (dim × 4 + 128)` — embedding bytes + key/overhead

### 4. Cache sits in server handler, not pipeline

The `Server` struct holds a `*Cache` (nil when disabled). Each handler checks the cache before calling `reg.GetOrInit`:

```
handleEMB:
  for each text:
    key = model + ":" + text
    if cache.Get(key) → collect hit
    else → track as miss
  if all hits → return immediately (no model load, no pool dispatch)
  if some misses → entry, err := reg.GetOrInit(model)
                    resp, err := entry.Pool.Embed(missTexts)
                    for each miss → cache.Set(key, embedding)
                    merge → return
```

Cache hits skip model loading entirely — a text that's cached doesn't trigger `GetOrInit`, so lazy-loaded models that only serve cached texts never load. This is a meaningful optimization for workloads with high repetition.

### 5. Size parsing with `github.com/docker/go-units`

The `FromHumanSize("1GB")` function handles `KB`, `MB`, `GB`, `TB`, decimal values (`1.5GB`), and raw bytes. Importing this avoids writing a fragile parser and is a well-known Go dependency with stable API.

### 6. Per-text caching, not per-request

`EMB model "hello" "world"` sends two texts. Caching per-text means individual texts that repeat across different request shapes still hit. The split-merge logic (cache hits → immediate, misses → batch → store → merge) handles this at the handler level.

### 7. Metrics in `EMB.INFO` and `EMB.STATS`

`EMB.INFO` adds six key-value pairs:
- `cache_hits`, `cache_misses`, `cache_hit_rate`, `cache_evictions`, `cache_entries`, `cache_max_entries`, `cache_memory_bytes`

`EMB.STATS` includes aggregate cache stats across all models (since the cache is global, this is straightforward).

### 8. Eviction: LRU, byte-budget based

When `curBytes` exceeds `maxBytes` after a `Set`, entries are evicted from the back of the list (least recently used) until `curBytes` is back under budget. Each eviction increments the eviction counter.

### 9. Thread safety

The cache uses `sync.Mutex` for the map + list operations. The critical section is <1µs (map lookup + list move to front). At 1000 req/s the lock is contended <0.1% of the time. Atomic counters for hits/misses/evictions avoid locking on read paths.

## Go Code Quality Tooling

The current `.golangci.yml` is minimal (govet + staticcheck). CI only runs `gofmt -l` and `go vet`, missing many issues that a linter like RuboCop catches for Ruby. Adding Go quality tooling ensures the new cache code and the broader codebase stay consistent and correct.

### Decisions

#### 1. Expand golangci-lint linters to match RuboCop's role

The equivalent of RuboCop for Go is `golangci-lint` with a robust set of linters. Enable:

| Linter | Purpose |
|--------|---------|
| `govet` | Suspicious constructs (already enabled) |
| `staticcheck` | Bugs, performance, simplicity (already enabled) |
| `errcheck` | Unchecked errors |
| `gosimple` | Simplify code |
| `ineffassign` | Unused assignments |
| `unused` | Unused code |
| `stylecheck` | Style consistency |
| `gosec` | Security issues |
| `revive` | Opinionated style rules |
| `gocritic` | Idiomatic Go patterns |

This mirrors RuboCop's role: catching bugs, enforcing style, and keeping the codebase consistent.

#### 2. Wire into CI

Add a `lint` step to `.github/workflows/ci.yml` that runs `golangci-lint run ./...` on every push/PR. The CI already has Go installed — just need to add `golangci-lint`.

#### 3. Fix all existing issues

Run `golangci-lint fmt` (formatting) and `golangci-lint run --fix` (auto-fix) across all Go packages, then manually fix any remaining issues. This ensures the new config passes cleanly with zero offenses.

### Risks / Trade-offs

- **New linters may produce false positives** — `gosec` in particular can flag non-issues in this codebase (e.g., no TLS). Mitigated by `nolint` directives or exclusion rules in `.golangci.yml`.
- **Auto-fix may change formatting** — reviewed before committing.
- **CI lint step adds ~30s to workflow** — runs in parallel with tests.

## Risks / Trade-offs

- **Container blind spot**: `totalSystemMemory()` reads host RAM, not cgroup limits. On a 64GB host with a 512MB container limit, `"auto"` could overallocate. Mitigated by the entries cap (100K max). Users in constrained environments should set an explicit size.
- **GC pressure**: Cache entries live on Go heap. At 100K entries × 384-dim (~170MB), the GC scans the heap. This is noise compared to the ~3-5ms inference time — the concurrent GC overhead is ~10-50µs per cycle.
- **Text normalization**: The cache key is the raw text string. `"hello"` and `"Hello "` are different cache entries. Normalization (trim, lowercase) could be added later but is intentionally out of scope for now.
- **No size limit on individual entries**: A 10KB text produces a 384-dim embedding (1.5KB). The embedding size is bounded by the model dimension, so this is fine — no single entry can blow the budget.
