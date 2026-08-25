## 1. Config

- [x] 1.1 `BatchingConfig.Timeout` → `*int` (nil = default-on 1ms, 0 = disabled, >0 = window)
- [x] 1.2 Registry defaults nil→1; wire resolved timeout through `NewPool`

## 2. Consumers

- [x] 2.1 Harness emits explicit `batching: timeout: 0` for pool-mode runs (baselines preserved)
- [ ] 2.2 README/config comments document default-on and the `timeout: 0` escape hatch

## 3. Spec & docs

- [x] 3.1 smart-batching spec delta: default-on + disabled scenarios

## 4. Validation stage (nix develop)

- [x] 4.1 `go build ./...` passes
- [x] 4.2 Bare config (no batching key) loads with `batching=1ms/32/16384` in the log; `timeout: 0` loads the pool
- [x] 4.3 `go test ./...` + `just lint` pass
- [ ] 4.4 Production config (siglip2 + hyperclusters) still byte-identical (validated earlier; re-run on gold host)