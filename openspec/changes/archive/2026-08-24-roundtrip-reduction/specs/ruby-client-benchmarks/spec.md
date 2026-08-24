# ruby-client-benchmarks

## Purpose

Specifies an end-to-end benchmark harness for the Ruby client (`gems/emb`): reproducible
scenarios measuring round-trip reduction (eager vs lazy batched vs pipelined), connection
pool behavior under concurrency, and a stability gate proving inference throughput stays
stable while the request path is CPU-busy.

## ADDED Requirements

### Requirement: End-to-end client benchmark harness

The gem SHALL provide a benchmark harness in `gems/emb/bench/`, runnable via
`just bench-ruby` and `rake bench`, that measures end-to-end embedding performance
against a local emb server. The harness SHALL cover sequential eager calls, lazy batched
calls (`Emb.batch`), eager-pipelined bursts, and multithreaded concurrency.

#### Scenario: Harness runs all scenarios

- **WHEN** `just bench-ruby` is run with the emb server available
- **THEN** each scenario SHALL report requests/sec, p50, and p99
- **THEN** the harness SHALL exit non-zero if any scenario fails to produce numbers

#### Scenario: Lazy batching reduces round trips

- **WHEN** the harness compares N eager single-text embeds against one lazy-batched use
  of N loaders
- **THEN** the lazy scenario SHALL send exactly one `EMB.MULTI` command for N pairs
- **THEN** the lazy scenario SHALL report lower or equal end-to-end latency than N eager
  round trips

### Requirement: Overhead ratio reporting

The harness SHALL report an overhead ratio per scenario: the end-to-end time per embed
relative to the warm inference baseline (single embed, cache warm), defined as
`(e2e − inference) / inference`. Results SHALL be published in `BENCHMARK.md`.

#### Scenario: Overhead ratio published

- **WHEN** a contributor reads `BENCHMARK.md`
- **THEN** they SHALL find the overhead ratio for the eager, lazy-batched, and
  pipelined scenarios on the reference machine

### Requirement: Stability gate

The harness SHALL measure inference p50/p99 while a synthetic parse-heavy load (commands
with many/large arguments) exercises the request path concurrently. The stability ratio
(`p99_with_load / p99_idle` on the reference machine) SHALL meet the threshold published
in `BENCHMARK.md`; the gate SHALL fail otherwise.

#### Scenario: Inference stable under request-path CPU load

- **WHEN** the harness runs the stability scenario with the documented concurrency
- **THEN** it SHALL report the stability ratio and whether it meets the published
  threshold
- **THEN** the harness SHALL exit non-zero when the gate fails

### Requirement: CPU-partitioned benchmark runs

The harness and its justfile target SHALL support running benchmarks under a fixed CPU
partition: the emb server in an app partition (`GOMAXPROCS` + `intra_op_threads` budget;
`taskset -c` on Linux) and the benchmark tooling (harness, parse-load generator, and
`redis-benchmark`) in a disjoint benchmark partition. Every result set published in
`BENCHMARK.md` SHALL state the partition layout (app vs benchmark) it was measured under.

#### Scenario: Partitioned run target

- **WHEN** `just bench-ruby` runs with the server in the app partition and the harness in
  the benchmark partition
- **THEN** the server SHALL start with the app-partition CPU budget
- **THEN** the harness SHALL run with the benchmark-partition budget

#### Scenario: Partition layout published

- **WHEN** a contributor reads `BENCHMARK.md`
- **THEN** they SHALL find the CPU partition layout (app vs benchmark) alongside the
  hardware line for every result set