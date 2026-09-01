# redis-benchmark-metrics

## Purpose

Specifies the use of `redis-benchmark` as the primary benchmark driver for RESP throughput measurements, with reproducible methodology and published results.

## Requirements

### Requirement: redis-benchmark as benchmark driver

The project SHALL use the standard `redis-benchmark` tool (from redis upstream) as the primary benchmark driver for RESP throughput measurements.

#### Scenario: redis-benchmark sends EMB command

- **WHEN** `redis-benchmark` is invoked with positional arguments `EMB minilm "hello world"`
- **THEN** it SHALL send properly formatted RESP `*3\r\n$3\r\nEMB\r\n$6\r\nminilm\r\n$11\r\nhello world\r\n` to the server
- **THEN** the server SHALL respond with an embedding vector (bulk string) and `redis-benchmark` SHALL count it as a successful request

#### Scenario: redis-benchmark output format

- **WHEN** `redis-benchmark` is run with `-q`
- **THEN** output SHALL be a single line per test: `EMB minilm hello world: XXXXX.XX requests per second`

### Requirement: Reproducible benchmark methodology

The project SHALL provide a single-command recipe to reproduce benchmark results, including server startup and benchmark execution.

#### Scenario: just bench-redis runs end-to-end

- **WHEN** user runs `just bench-redis` (or `just bench-all`, which composes `bench-redis` + `bench-cache`)
- **THEN** the server SHALL start, `redis-benchmark` SHALL run, the server SHALL stop
- **THEN** benchmark output SHALL be printed to stdout

#### Scenario: Single-threaded benchmark

- **WHEN** the server runs with `GOMAXPROCS=1` (`GOMAXPROCS=1 ./bin/emb -config config.yaml`)
- **THEN** `redis-benchmark -p 6379 -q -c 1 -P 1 -n 500 EMB minilm "hello world"` SHALL produce a valid requests/sec result
- **THEN** the same holds for the higher-concurrency cells used by `just bench-redis-single` (`-c 8/16 -P 1 -n 2000`, `-c 1 -P 8 -n 2000`) — see BENCHMARK.md

#### Scenario: Multi-threaded benchmark

- **WHEN** the server runs with `GOMAXPROCS=0` (all cores)
- **THEN** `redis-benchmark -p 6379 -q -c 16 -P 1 -n 2000 EMB minilm "hello world"` SHALL produce a valid requests/sec result
- **THEN** the same holds for the cells used by `just bench-redis-multi` (`-c 1/8/64 -P 1 -n 500/2000`) — see BENCHMARK.md
- **NOTE** the high-pipeline invocations of the original proposal (`-P 512 -c 512 -n 100000/-n 1000000`) are intentionally NOT used: BENCHMARK.md documents that `-P 512` queues hundreds of inferences behind a single worker and produces misleading throughput numbers.

### Requirement: Published benchmark results

The project SHALL publish benchmark results in BENCHMARK.md following the tidwall/redcon format.

#### Scenario: BENCHMARK.md contains methodology

- **WHEN** a contributor reads BENCHMARK.md
- **THEN** they SHALL find the exact commands needed to reproduce results for each configuration
- **THEN** they SHALL find the server startup command and `redis-benchmark` invocation

#### Scenario: BENCHMARK.md contains results

- **WHEN** a contributor reads BENCHMARK.md
- **THEN** they SHALL find requests/sec numbers for each tested configuration

### Requirement: emb-bench deprecated (removed)

The hand-rolled `emb-bench` Go benchmark tool SHALL NOT exist in the repository; `redis-benchmark` is the sole benchmark driver (completed deprecation by removal).

#### Scenario: emb-bench is gone

- **WHEN** a contributor searches the repository or the `justfile` for the `emb-bench` benchmark command
- **THEN** they SHALL not find it — the tool was removed and `redis-benchmark` became the sole driver (see the first requirement of this spec)
