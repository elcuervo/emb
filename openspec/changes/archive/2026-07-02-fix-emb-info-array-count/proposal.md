## Why

`EMB.INFO` has a hardcoded `WriteArray(28)` in the server, but the actual number of key-value pairs varies based on whether caching is enabled (22 without cache, 36 with cache). The mismatch causes the Redis client to wait for phantom array elements, timing out with `ReadTimeoutError`. This breaks the release CI pipeline.

## What Changes

- Fix `handleINFO` to write the correct array count matching the actual number of elements sent
- Add Go tests for `handleINFO` response parsing to prevent regression

## Capabilities

### New Capabilities
- `emb-info-array-count`: Correct array length in EMB.INFO response

### Modified Capabilities
- *(none — fixing a bug, not changing requirements)*

## Impact

- `internal/server/server.go`: Fix `WriteArray` count in `handleINFO`
- `internal/server/server_test.go`: Add test coverage for INFO response parsing
- Ruby gem tests in CI will no longer time out on `EMB.INFO`
