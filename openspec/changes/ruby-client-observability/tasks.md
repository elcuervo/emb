## 1. Configuration

- [ ] 1.1 Add `logger`, `log_level`, `metrics`, `debug_payload` to `Configuration` OPTIONS + defaults
- [ ] 1.2 `Emb.debug!` maps to log-level debug (keeps public API)

## 2. Typed stats & info

- [ ] 2.1 New `Emb::Types` with INTEGER/FLOAT/BOOL field tables
- [ ] 2.2 `Client#coerce` applies the table; unknown keys pass as String
- [ ] 2.3 `info`/`stats` return typed values; `models` unchanged

## 3. Logging

- [ ] 3.1 `send_command` debug output → `logger.debug` with sanitized fields (command, arg count, text bytes, ms)
- [ ] 3.2 `debug_payload: true` includes full texts (documented PII note)
- [ ] 3.3 Logger path covers `batch`/`multi` (they share `send_command`)

## 4. Client-side metrics

- [ ] 4.1 `Client::Metrics` ring-buffer accumulator (count, mean, p50/p99, bytes), thread-safe
- [ ] 4.2 `metrics: true` opt-in; off = no per-call bookkeeping
- [ ] 4.3 `Client#metrics` + `Emb.metrics` typed hash

## 5. Bench output

- [ ] 5.1 `EMB_BENCH_OUTPUT=json|csv` emitters for scenario rows
- [ ] 5.2 Stability gate verdict included in machine output

## 6. Validation stage

- [ ] 6.1 `bundle exec rake` (server on test-two-models.yaml) passes incl. new specs
- [ ] 6.2 New specs: logger routing, sanitization, typed info/stats, metrics accumulation
- [ ] 6.3 `bundle exec rubocop` clean
- [ ] 6.4 `EMB_BENCH_OUTPUT=json` produces valid JSON (req/s, p50, p99, stability verdict)
- [ ] 6.5 README (gems/emb) documents logger/metrics/typed values