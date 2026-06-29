## 1. Config

- [x] 1.1 Add `Preload` (bool) and `Workers` (int) fields to `config.ModelConfig`
- [x] 1.2 Update config validation: no new required fields (both optional with zero defaults)

## 2. Observability — Wire Counters

- [x] 2.1 Add `Requests() int64` and `AvgLatency() float64` methods to `Worker`
- [x] 2.2 Add `Requests() int64` and `AvgLatency() float64` methods to `Pool` (aggregates workers)
- [x] 2.3 Update `EMB.INFO` handler to read real stats from `ModelEntry.Pool`
- [x] 2.4 Update `EMB.STATS` to include per-model request breakdown

## 3. Memory Auto-Tune

- [x] 3.1 Implement `totalSystemMemory() uint64` for Darwin (`sysctl hw.memsize`)
- [x] 3.2 Implement `autoTuneWorkers(modelPath string, maxWorkers int) int` formula
- [x] 3.3 Wire auto-tune into pool creation: use `cfg.Workers` if > 0, else auto-tune
- [x] 3.4 Add `golang.org/x/sys/unix` dependency to go.mod

## 4. Lazy Loading

- [x] 4.1 Refactor `ModelEntry` to hold config for lazy init (`cfg`, `name`, `sync.Once`)
- [x] 4.2 Implement `ensurePool()` method on `ModelEntry` (creates pool on first call)
- [x] 4.3 Add `Registry.GetOrInit(name string) (*ModelEntry, error)` method
- [x] 4.4 Update `handleEMB` to use `reg.GetOrInit` instead of `reg.Get`
- [x] 4.5 Update `cmd/emb/main.go`: only preload models with `preload: true`

## 5. Tests and Verification

- [x] 5.1 Run `just test` and confirm all tests pass
- [x] 5.2 Run `just bench` and confirm no regression vs baseline
- [x] 5.3 Start server with `preload: false` (default) and verify model loads on first EMB
- [x] 5.4 Verify `EMB.INFO` returns real request count and latency (requests: 1, avg_latency_us: 2437)
- [x] 5.5 Verify `EMB.STATS` shows per-model breakdown (minilm:1)
- [x] 5.6 Verify `go vet ./...` passes
