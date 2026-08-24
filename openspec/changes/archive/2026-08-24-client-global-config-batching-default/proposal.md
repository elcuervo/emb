## Why

The `emb` Ruby gem's defaults are hardcoded and there is no way to configure it globally.
`Client` ships `DEFAULTS = { pool: 5 }`, batching is off (`batch: false`), the RESP driver
defaults to pure-Ruby, and the only global knob is the `EMB_URL` env var. Every other
setting must be repeated at every `Emb.setup(...)` / `Emb.new(...)` call site, so real
deployments drift from the tuned settings and cannot flip batching on without editing each
call.

The benchmark evidence (roundtrip-reduction + server-inference-isolation) is decisive for
the out-of-the-box defaults: lazy batching collapses N eager round trips into one
`EMB.MULTI` at equal throughput with a tight p99 (no tail), the connection-pool sweep kept
the default at 5, and the pure-Ruby driver stays default over `hiredis`. We want those
evidence-based choices to be the **default for every client**, and a simple, **single way
to configure the gem once** — `Emb.configure` — while explicit per-call options keep
winning. `EMB_URL` stays the only environment variable.

## What Changes

- **Global configuration** in the `emb` gem: `Emb.configure { |c| ... }` sets defaults
  once and every client created afterwards (`Emb.setup`, `Emb.new`, and the lazy default
  client) inherits them; `Emb.defaults`/`Emb.configuration` for reading/resetting.
- **Batching on by default**: the lazy proxy path (`batch: true`) becomes the
  out-of-the-box default so every embed coalesces into `EMB.MULTI`; switchable back to
  eager via `Emb.configure { |c| c.batch = false }` or per call (`batch: false`).
- **Best out-of-the-box config** (evidence-based): `batch: true`, `pool: 5`, pure-Ruby
  driver, `protocol: 2`, `reconnect_attempts: 3`, sensible timeouts — all overridable via
  `Emb.configure`, documented in the README and `BENCHMARK.md`.
- **Precedence**: explicit per-call args > `EMB_URL` (url only) / `Emb.configure` values >
  built-in defaults.

## Capabilities

### New Capabilities

- `client-global-configuration`: module-level global config for the `emb` gem via
  `Emb.configure` / `Emb.defaults`, precedence rules, and the evidence-based
  out-of-the-box defaults (batching on, pool 5, pure-Ruby driver).

### Modified Capabilities

- `emb-ruby-client`: the proxy API's default becomes lazy batching (`batch: true`) instead
  of `batch: false`, configurable via `Emb.configure`; connection-pool and driver selection
  become globally configurable via `Emb.configure`.

## Impact

- **Gem**: `gems/emb` — `lib/emb/configuration.rb` (new), `client.rb` wiring
  (batch/pool/driver/timeouts from global config), `emb.rb` (`Emb.configure`/`Emb.defaults`),
  README configuration section.
- **Tests**: unit tests for `Emb.configure`, precedence, and the new default batching
  behavior (lazy default, eager opt-out).
- **Docs**: `BENCHMARK.md` (out-of-the-box config + rationale), gem README.
- **Behavior**: default proxy calls become lazy (create-then-consume contract); existing
  eager usage can opt back in globally or per call.