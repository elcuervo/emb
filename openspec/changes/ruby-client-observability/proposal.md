## Why

The Ruby client's observable surface hasn't kept up with the server's. Three concrete problems in the current code:

1. **Debugging output is unstructured and unsafe.** `Client#send_command` (client.rb:24-34) prints `$stdout.puts "[EMB] #{args.inspect} (#{ms}ms)"` only when a global `Emb.debug!` flag is set. It goes to **stdout** (polluting app output, not capturable through a logger), prints **full text payloads** (`args.inspect` leaks sent texts/PII), and has no way to configure where it goes or what it includes.

2. **`stats`/`info` return the wrong data for performance analysis.** `Emb.stats` returns the raw RESP array of **strings**; `Emb.info` (client.rb:57-65) turns key/value pairs into a hash but leaves every value as a String (`{dim: "384", requests: "42", padding_efficiency: "0.8712"}`). None of the server's new performance fields — `batching_timeout_ms`, `batching_max_batch`, `batching_max_tokens`, `padding_efficiency`, `model_bytes`, `quantization`, `avg_latency_us`, `tokens` — are typed, so an app cannot compute with them without manual conversion.

3. **No client-side latency visibility.** Timing only happens under the debug flag and is thrown away after the print. There is no per-model request/latency/byte accumulator comparable to the server's `EMB.STATS`, and `bench/bench.rb` only prints a human table (no machine-readable output from the raw-XML/JSON the harnesses use).

## What Changes

- **Structured logging.** Add `logger` (default `Logger.new(IO::NULL)`) and `log_level` to `Emb::Configuration`. `send_command` debug output moves to `logger.debug`, includes command, arg/text **counts** and **byte sizes** instead of full payloads (full text only with an opt-in `debug_payload: true`), and the round-trip time. `Emb.debug!` continues to work (maps to `logger.level = :debug`) but the output flows through the configured logger.
- **Typed `info`/`stats`.** Introduce a server-field schema mapping RESP strings to Ruby types for the known keys (`dim`→Integer, `requests`→Integer, `avg_latency_us`→Integer, `tokens`→Integer, `errors`→Integer, `batching_timeout_ms`→Integer, `batching_max_batch`→Integer, `batching_max_tokens`→Integer, `padding_efficiency`→Float, `model_bytes`→Integer, `max_length`→Integer, `workers`→Integer, `normalize`→Integer/Boolean, `pooling`→String, `quantization`→String, …). Unknown keys pass through as String (server/version-forward compatible). `models` keeps its typed shape.
- **Client-side metrics (optional).** A `metrics: true` config toggle accumulates per-model counters in the client: request count, total/mean latency, p50/p99 (bounded ring buffer), and response bytes. Exposed as `Emb.metrics` / `Client#metrics` returning a typed hash. Default `off` so the hot path stays zero-cost.
- **Machine-readable bench output.** `bench/bench.rb` gains `EMB_BENCH_OUTPUT=json|csv` (default table) emitting scenario results plus the stability gate result for dashboards.

## Capabilities

### New Capabilities

- `ruby-client-observability`: structured debug logging, typed server stats for the Ruby client, optional client-side latency metrics, and machine-readable bench output.

### Modified Capabilities

- `emb-ruby-client`: `stats`/`info` return typed values; logging is configurable.

## Impact

Files (all under `gems/`): `emb/lib/emb/configuration.rb` (+`logger`, `log_level`, `metrics`, `debug_payload`), `emb/lib/emb/client.rb` (logger channel, metrics accumulator, typed RESP parsing), `emb/lib/emb/types.rb` (new: field→type schema + typed parse), `emb/lib/emb/multi.rb` + `batch.rb` (inherit logger/metrics), `emb/lib/emb.rb` (module accessors), `emb/bench/bench.rb` (output formats), specs/tests and READMEs. No server changes, no protocol changes, no behavior change to `Embed` return types.

## Validation

- Test suite (`bundle exec rake`) passes; new specs cover logger routing, payload sanitization, typed `info`/`stats`, and metrics accumulation.
- `EMB_BENCH_OUTPUT=json` produces valid JSON with per-scenario req/s, p50, p99, and the stability gate verdict.
- Byte-for-byte identical `info`/`stats` values to today, just typed.
- `just lint` (rubocop) passes.