## 1. Server fix

- [x] 1.1 Fix `WriteArray` count in `handleINFO` to match actual elements (22 without cache, 36 with cache)
- [x] 1.2 Add RESP parser helper that reads a complete RESP response (array header + all elements)
- [x] 1.3 Add `TestServerINFOArrayCount` that parses full EMB.INFO response and validates 22 elements without cache
- [x] 1.4 Add `TestCacheInfoArrayCount` that parses full EMB.INFO response and validates 36 elements with cache

## 2. Verify

- [x] 2.1 Run Go tests: `go test -v -run INFO ./internal/server/` — 4 tests pass including new count validation
- [x] 2.2 Run full Go server test suite — 32/32 pass, no regressions
