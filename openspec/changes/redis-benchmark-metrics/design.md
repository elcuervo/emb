## Context

The emb server speaks the RESP (Redis Serialization Protocol) wire protocol over TCP. The standard `redis-benchmark` tool accepts positional arguments and formats them as RESP commands via `redisFormatCommandArgv`, making it a drop-in benchmarking driver for any RESP-compatible server. The tidwall/redcon project demonstrates this pattern: it publishes `redis-benchmark -p <port> -t set,get -n 10000000 -q -P 512 -c 512` invocations and reports the raw `requests per second` output.

Currently, emb has:
- A hand-rolled `emb-bench` Go tool (50 sequential requests, reports P50/P95/P99 in ms)
- Go unit benchmarks (`BenchmarkRESP`, `BenchmarkPoolEmbed`, `BenchmarkMeanPool`)
- No `redis-benchmark` integration
- No standardized benchmark methodology comparable to redcon's

## Goals / Non-Goals

**Goals:**
- Use `redis-benchmark` (Homebrew-installable) as the sole benchmark driver for the RESP server
- Publish BENCHMARK.md following redcon's format: setup command, `redis-benchmark` invocation, `requests per second` results
- Add `just bench-redis` recipes that wrap server startup + benchmark + teardown
- Deprecate `cmd/emb-bench` in favor of the standard tool
- Benchmark configurations: single-threaded (GOMAXPROCS=1), multi-threaded (GOMAXPROCS=0), with and without batching, various concurrencies (`-c`) and pipeline depths (`-P`)

**Non-Goals:**
- NOT modifying `redis-benchmark` itself
- NOT benchmarking model inference directly (covered by Go unit benchmarks)
- NOT benchmarking multi-model or `EMB.MULTI` via `redis-benchmark` (single static command)
- NOT running benchmarks in CI on every commit (opt-in / nightly at most)

## Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Benchmark driver | `redis-benchmark` from Homebrew | Matches redcon's exact methodology. `redis-benchmark` accepts arbitrary RESP commands as positional args. No custom Go tool needed. |
| EMB command payload | `EMB minilm hello world` | Static text payload of realistic size (~11 chars of text). `redis-benchmark` sends the same text for every request in a given run. |
| Output format | `requests per second` with `-q` flag | Quiet mode produces a single line per test (`EMB minilm hello world: XXXXXX requests per second`), matching redcon's format exactly. |
| Server startup | `just bench-redis` recipe handles start/wait/bench/stop | Single entry point, reproducible across machines. Server logs go to a temp file, stdout stays clean for benchmark output. |
| emb-bench fate | Deprecated but kept in tree | The hand-rolled bench still measures P50/P95/P99 in ms (different dimension from req/s). Useful for latency distribution debugging. Move to `cmd/emb-bench/deprecated/` or add `DEPRECATED` notice. |

## Risks / Trade-offs

- [Static payload] `redis-benchmark` sends the same command bytes for every request. `EMB minilm hello world` always embeds "hello world", so tokenizer output is deterministic. This is fine for throughput benchmarking — real-world usage also sees repeated text patterns.
- [No batching timing] `redis-benchmark`'s pipelining (`-P`) sends multiple commands before reading replies. This tests RESP I/O throughput but doesn't exercise emb's server-side batching (which batches *concurrent* requests by timeout). To test server-side batching, increase `-c` (connections) so requests pile up and the batcher flushes.
- [Model dependency] Results depend on the ONNX model used. Always note the model and hardware in BENCHMARK.md.
