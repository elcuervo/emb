## 1. Handler

- [x] 1.1 Register `emb.multi` in server mux
- [x] 1.2 Implement `handleEMBMULTI`: parse pairs, validate even count, write error for wrong args
- [x] 1.3 Fan-out: goroutine per pair calling `reg.GetOrInit` then `Pool.Embed`
- [x] 1.4 Collect results by index via `sync.WaitGroup`
- [x] 1.5 Write array response with nil for failures

## 2. Stats

- [x] 2.1 Each successful pair increments `s.total` by 1 in the goroutine
- [x] 2.2 Verify `EMB.STATS` reflects per-pair counting

## 3. Documentation

- [x] 3.1 Add `EMB.MULTI` to `EMB.HELP` output

## 4. Tests

- [x] 4.1 Test: single pair returns array of one bulk string
- [x] 4.2 Test: multiple ordered pairs return ordered array
- [x] 4.3 Test: odd argument count returns error
- [x] 4.4 Test: too few arguments returns error
- [x] 4.5 Test: unknown model returns nil at that position
- [x] 4.6 Test: stat counters increment correctly
- [x] 4.7 Run `go test ./...` — all tests pass

## 5. E2E Tests

- [x] 5.1 Add `verify-emb-multi` justfile recipe that downloads two models and configures both
- [x] 5.2 Create `cmd/emb-multi-verify/main.go`: connects to server, sends EMB.MULTI across models, validates response structure
- [x] 5.3 Verify same-model pairs get batched into fewer ONNX runs
- [x] 5.4 Run `just verify-emb-multi` — all checks pass (7/7 e2e tests)
