## Context

The Ruby client (`gems/emb`) is the primary server consumer in the unsplash stack. Its debug surface is `client.rb:24-34` — a hard-coded `$stdout.puts` behind a global `Emb.debug?` flag that prints full argument lists (text payloads) and a timestamped ms figure that is discarded immediately. `Emb.info`/`Emb.stats` slice RESP key/value pairs but leave values as strings, so the newer server performance fields (`padding_efficiency`, `batching_max_tokens`, `model_bytes`, `quantization`, `avg_latency_us`) are unusable without manual conversion. `bench/bench.rb` already computes solid numbers (req/s, p50/p99, overhead, stability gate) but only prints a table.

## Goals / Non-Goals

**Goals:**
- Logging through a configurable logger, debug output sanitized by default
- `info`/`stats` return typed Ruby values for known server fields; unknown fields passthrough as String
- Optional client-side per-model metrics (count, mean, p50/p99, bytes); zero-cost when off
- `EMB_BENCH_OUTPUT=json|csv` for bench.rb
- No change to embed return values (arrays of floats / arrays of arrays)

**Non-Goals:**
- Changing the RESP protocol or server commands
- Server-side changes (fields already exist in `EMB.INFO`/`EMB.STATS`)
- Distribution of metrics (Push not included — app polls `Emb.metrics`)
- Ruby gems other than `emb` (`emb-server` is a binary launcher; unchanged)

## Decisions

### Logger on the Configuration value object
`logger` (default `Logger.new(IO::NULL)`) and `log_level` join the existing `Configuration` `OPTIONS` list so per-call/client config can override, matching the existing resolution order (explicit → `Emb.configure` → default). `Emb.debug!` becomes `self.logger.level = Logger::DEBUG` on the configuration, keeping the public API stable.

### Sanitization: counts and bytes, not texts
Debug lines log `command`, `arg count`, `total text bytes`, `round-trip ms`; the full text array appears only with `debug_payload: true`. This fixes the current text leak while staying useful for latency/traffic debugging (which never needs the text).

### Typed parsing lives in `Emb::Types` with a field→type table
```
INTEGER = %i[dim max_length workers requests avg_latency_us tokens errors
             batching_timeout_ms batching_max_batch batching_max_tokens model_bytes]
FLOAT   = %i[padding_efficiency]
BOOL    = %i[normalize]
```
`Client#coerce(key, value)` applies the table; anything else stays a String (version-forward). This makes existing callers that already convert (`v.to_i`) unaffected, and new ones get correct types for free.

### Metrics: a lightweight ring buffer, opt-in
`metrics: true` keeps per-model `[count, total_us, ring(bytes), ring(ms)]`; p50/p99 from a fixed-size ring (e.g., 256 samples) — cheap and bounded. Off by default so the eager hot path allocates nothing extra. Thread-safe via a `Mutex` (pool lookups are already synchronized; metrics are read far less often than written).

### bench.rb: format switch before printing
`EMB_BENCH_OUTPUT` selects the emitter: table (default), `json`, or `csv`. JSON includes scenario rows + stability gate verdict, matching the fargate harness convention of machine-readable baselines.

## Risks / Trade-offs

- [Debug default off / typed values change nothing for current callers] → additive surface; `info` type change could technically break someone doing `info[:dim] + "px"` string concat — acceptable, the API doc will state types.
- [Ring buffer memory] → bounded (256 samples × concurrent models), negligible.
- [`debug_payload: true` re-enables text logging] → explicit opt-in, documented as PII-sensitive.

## Migration Plan

1. `Configuration` + `OPTIONS` gain `logger`, `log_level`, `metrics`, `debug_payload`.
2. `Emb::Types` + `Client#coerce`; `info`/`stats` use it.
3. `send_command` routes through logger (debug channel, sanitized fields, always-on timing when `metrics: true`).
4. `Client#metrics` accumulator + module accessor `Emb.metrics`.
5. `bench/bench.rb` format switch.
6. Specs (RSpec), rubocop, README update.

## Open Questions

- Should `metrics` default to ON for `Emb.batch` loaders (they aggregate many embeds per flush) or stay strictly per-command? (Proposal: count aggregate pairs, not just commands, when metrics are on.)