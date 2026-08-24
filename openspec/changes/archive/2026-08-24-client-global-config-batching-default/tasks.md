# Tasks: Client Global Config + Batching Default

## 1. Configuration layer

- [x] 1.1 Add `gems/emb/lib/emb/configuration.rb` with an `Emb::Config`
  (`Struct.new(..., keyword_init: true)`) carrying `host`, `port`, `url`, `pool` (5),
  `batch` (true), `driver`, `protocol` (2), `connect_timeout`, `read_timeout`,
  `write_timeout`, `reconnect_attempts` (3) plus a `Config.defaults` constructor, and wire
  it into `lib/emb.rb` (`Emb.configure { |c| ... }`; `Emb.configuration`)
- [x] 1.2 No per-setting env vars: `EMB_URL` remains the only env var (read in the
  client, existing behavior); everything else flows through `Emb.configure` or built-ins

## 2. Client wiring

- [x] 2.1 In `Emb::Client#initialize`, merge `Emb.configuration.defaults` with explicit
  args (explicit wins) and apply `batch`, `pool`, `driver`, `protocol`, timeouts,
  `reconnect_attempts`, `host`/`port`/`url` to `ConnectionPool` / `RedisClient`;
  `EMB_URL` fallback for url preserved
- [x] 2.2 Change the `batch` default to `true` (out-of-the-box lazy batching); keep
  global (`Emb.configure`) and per-call `batch: false` opt-out working

## 3. Tests

- [x] 3.1 Unit tests: `Emb.configure` then `Emb.new` inherits (pool, batch, driver);
  per-call option beats `Emb.configure`; `EMB_URL` still the only env var affecting
  connection; `Emb.configuration` reflects configured + built-ins
- [x] 3.2 Behavior tests: default client is lazy (no command until value used);
  `batch: false` (global via `Emb.configure` and per call) returns to eager; lazy result
  equals eager result
- [x] 3.3 Update `emb_spec.rb`/`emb_batch_spec.rb` for the new default (batching on) without
  breaking existing eager scenarios (explicit `batch: false` where eager is asserted)

## 4. Docs

- [x] 4.1 Gem README: a Configuration section (`Emb.configure`/`Emb.configuration`, precedence,
  out-of-the-box defaults + rationale, eager opt-out, `EMB_URL` note)
- [x] 4.2 `BENCHMARK.md`: document the out-of-the-box client config (batching on, pool 5,
  pure-Ruby driver) and the benchmark rationale

## 5. Validation

- [x] 5.1 rubocop clean, `bundle exec rake spec` green, `openspec validate
  client-global-config-batching-default --type change` passes
