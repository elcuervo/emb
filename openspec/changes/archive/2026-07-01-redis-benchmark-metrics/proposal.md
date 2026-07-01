## Why

The project has existing Go unit benchmarks (`BenchmarkRESP`, `BenchmarkPoolEmbed`) and a hand-rolled e2e benchmark (`emb-bench`), but it lacks standardized, reproducible methodology using the industry-standard `redis-benchmark` tool. The tidwall/redcon benchmarks do exactly this:

```
redis-benchmark -p 6380 -t set,get -n 10000000 -q -P 512 -c 512
SET: 2018570.88 requests per second
GET: 2403846.25 requests per second
```

emb speaks RESP, so `redis-benchmark` can send `EMB` commands directly via its positional-arg mode:

```
redis-benchmark -p 6379 -n 10000000 -q -P 512 -c 512 EMB minilm hello world
```

No custom tool, no Lua scripts — the same binary Redis users already know.

Without this, there's no way to:
- Compare emb's throughput against other RESP-compatible servers using identical tooling
- Track performance regressions across releases with a one-liner
- Give users a clear picture of expected performance (req/s under specific conditions)
- Reproduce results consistently across machines

## What Changes

- Ensure `redis-benchmark` (via Homebrew) works as the benchmark driver — install it in CI and document in dev setup
- Remove or deprecate the hand-rolled `emb-bench` since `redis-benchmark` now covers the same ground with better methodology
- Create `BENCHMARK.md` documenting exactly how redcon does it: the setup command, the `redis-benchmark` invocation, and the results (just `requests per second` numbers for each config)
- Add `just bench-redis` recipe that wraps server startup + benchmark + teardown
- Run against multiple configs: single-threaded (`GOMAXPROCS=1`), multi-threaded (`GOMAXPROCS=0`), various concurrencies (`-c`), pipelining (`-P`), batching on/off

## Capabilities

### New Capabilities

- `redis-benchmark-metrics`: Standardized benchmark methodology following redcon's format — one-liner `redis-benchmark` commands, published `requests per second` results, reproduceable via `just bench-redis`

## Impact

| File | Change |
|------|--------|
| `BENCHMARK.md` | New file: methodology, reproduce commands, published results for emb vs baseline configs |
| `justfile` | Add `bench-redis` recipe(s): `just bench-redis` (single config), `just bench-redis-all` (all configs) |
| `cmd/emb-bench/main.go` | Deprecate/remove — replaced by `redis-benchmark` |
| `cmd/emb-verify/main.go` | Unchanged (still useful for correctness) |
| `README.md` | Add benchmark badge or link to BENCHMARK.md |
| `.github/workflows/ci.yml` | (optional) Add nightly benchmark job |
