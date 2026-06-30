## ADDED Requirements

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

- **WHEN** user runs `just bench-redis` or `just bench-redis-all`
- **THEN** the server SHALL start, `redis-benchmark` SHALL run, the server SHALL stop
- **THEN** benchmark output SHALL be printed to stdout

#### Scenario: Single-threaded benchmark

- **WHEN** the server runs with `GOMAXPROCS=1`
- **THEN** `redis-benchmark -n 100000 -q -P 512 -c 512 EMB minilm hello world` SHALL produce a valid requests/sec result

#### Scenario: Multi-threaded benchmark

- **WHEN** the server runs with `GOMAXPROCS=0` (all cores)
- **THEN** `redis-benchmark -n 1000000 -q -P 512 -c 512 EMB minilm hello world` SHALL produce a valid requests/sec result

### Requirement: Published benchmark results

The project SHALL publish benchmark results in BENCHMARK.md following the tidwall/redcon format.

#### Scenario: BENCHMARK.md contains methodology

- **WHEN** a contributor reads BENCHMARK.md
- **THEN** they SHALL find the exact commands needed to reproduce results for each configuration
- **THEN** they SHALL find the server startup command and `redis-benchmark` invocation

#### Scenario: BENCHMARK.md contains results

- **WHEN** a contributor reads BENCHMARK.md
- **THEN** they SHALL find requests/sec numbers for each tested configuration

### Requirement: emb-bench deprecated

The hand-rolled `emb-bench` Go benchmark tool SHALL be deprecated in favor of `redis-benchmark`.

#### Scenario: emb-bench deprecation notice

- **WHEN** a contributor reads `cmd/emb-bench/main.go`
- **THEN** they SHALL find a deprecation notice recommending `redis-benchmark` as the replacement
