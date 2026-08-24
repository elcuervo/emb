# Design: Client Global Config + Batching Default

## Context

`gems/emb` currently hardcodes defaults: `DEFAULTS = { host:, port:, pool: 5 }` in
`client.rb`, `batch: false`, pure-Ruby driver (`redis-client` default), and only `EMB_URL`
is read from the environment. `Emb.setup`/`Emb.new` take keyword args that pass through to
`RedisClient`. There is no shared configuration object, so tuned settings must be repeated
per call site.

Benchmark evidence (roundtrip-reduction + server-inference-isolation, v0.2.2): lazy
batching (one `EMB.MULTI`) is the round-trip win — equal throughput, p99 ≈ p50 (no tail);
the pool sweep kept 5; pure-Ruby driver stays default (hiredis below the 15% gate).

## Goals / Non-Goals

**Goals:**
- A single global configuration source (`Emb.configure`) feeding every client.
- Batching ON by default (evidence-based out-of-the-box), globally + per-call toggleable.
- Documented precedence: explicit args > `EMB_URL`/configured defaults > built-in defaults.
- `EMB_URL` remains the only environment variable.
- No behavior change for explicit eager usage.

**Non-Goals:**
- Auto-mounting `Emb::Middleware` (app concern; documented separately).
- Changing the server side or the `Emb.batch`/`Emb.multi` explicit APIs.
- A pool of env vars, per-setting env knobs, or machine-relative auto-tuning.

## Decisions

### D1: A minimal config value with a block-based configure

Add `Emb::Config` (new `lib/emb/configuration.rb`) as a one-line `Struct.new(..., keyword_init: true)`
with a `Config.defaults` constructor holding the built-in defaults.
`Emb.configure { |c| c.pool = 8 }` yields/mutates the shared config; `Emb.configuration`
reads it. `Emb.setup`/`Emb.new` merge `Emb.configuration` under their explicit kwargs, so
every client inherits global config.

Fields and built-ins: `host` `localhost`, `port` `6379`, `url` `nil`, `pool` `5`,
`batch` `true`, `driver` `nil` (redis-client default pure-Ruby), `protocol` `2`,
`connect_timeout`/`read_timeout`/`write_timeout` `nil` (redis-client defaults),
`reconnect_attempts` `3`.

### D2: EMB_URL stays the only environment variable

No env var per setting. `EMB_URL` keeps its existing behavior (connection URL fallback)
and is read in `Client#extract_url!`; a `url` set via `Emb.configure` takes precedence
over it, and an explicit `url:` per call wins over both. Everything else is configured via
`Emb.configure` or built-ins.

### D3: Batching on by default

Change the `batch` default to `true`. A freshly created client (no `batch` arg, no global
override) is lazy: `client[:minilm]["text"]` returns a batch-loader value sent as one
`EMB.MULTI` on first use (per `ruby-batch-loading`). Opt-out globally
(`Emb.configure { |c| c.batch = false }`) or per call (`Emb.new(batch: false)`).

- **Risk:** changes behavior for existing eager users. -> Mitigation: single clear opt-out
  via `Emb.configure`/call; README documents the create-then-consume contract and
  `Emb::Middleware` guidance for request-shaped apps.

### D4: Forward global settings

`pool`, `batch`, `driver`, `protocol`, timeouts, `reconnect_attempts`,
`host`/`port`/`url` all flow from the merged config into `ConnectionPool` /
`RedisClient`. Values left nil keep redis-client's own defaults, making the global layer a
drop-in for what today must be repeated per call.

## Risks / Trade-offs

- **[Behavior change: lazy by default]** Code that embeds and immediately uses a single
  value works identically; the risk is code that relies on the *side effect* of an
  immediate `EMB` (e.g., fire in a loop then consume later — one MULTI instead of N EMBs, a
  net win). -> Mitigation: documented opt-out and the create-then-consume contract.
- **[Hidden global state]** A singleton default can surprise after library load.
  -> Mitigation: `Emb.configure` is explicit and inspectable via `Emb.configuration`; per-call
  always wins.

## Migration Plan

- Land `Emb::Configuration` + `Emb.configure` first (non-breaking:
  defaults still `batch: false` at first, pool 5).
- Flip the `batch` default to `true` + tests for lazy default / eager opt-out, in one commit.
- Docs (README config section, `BENCHMARK.md` out-of-the-box config) with the benchmark
  rationale; bump version.