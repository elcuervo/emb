# Changelog

All notable changes to the `emb` gem are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [0.4.3.pre4] - unreleased

### Changed

- `lazy: :batch` now splits a resolving scope into one plain `EMB <model> <text>...`
  share per model and dispatches every share concurrently; it never sends
  `EMB.MULTI`. A two-model scope runs concurrently even when it fits in a single
  `batch_size` chunk. `lazy: :multi` still coalesces into serial `EMB`/`EMB.MULTI`.

### Added

- Optional OpenTelemetry instrumentation. With `opentelemetry-api` (and
  `opentelemetry-metrics-api` for metrics) installed, the client emits `emb.embed`,
  `emb.batch`, and `emb.share` spans plus `emb.client.*` metrics: per-call latency,
  deferral queue time, batch shape, share durations, observed parallelism, and
  in-flight shares. There is **no new runtime dependency** — the gems are loaded on
  first use, and without them every path returns immediately — and no emb-specific
  configuration; `OTEL_SDK_DISABLED=true` is the off switch. See `TELEMETRY.md`,
  including the view that applies the declared 1 ms–30 s histogram boundaries.
  Measured cost: ~1.1 µs per call with the API loaded and no SDK (~0.1% of a
  single-text round trip), and one branch without it.
- `eval`/`evalsha` honor the configured `lazy` mode: deferred under `:multi` and
  `:batch` (each call is its own share, concurrent in `:batch`) and eager under
  `false`. `decode:` applies when the value resolves.

### Breaking

- Under `lazy: :batch`, a share that fails terminally no longer fails the force for
  its sibling shares, and its items raise that share's `Emb::ServerError` on every
  use instead of resolving to `[]`. Healthy shares in the same scope resolve
  normally. The serial (`:multi`, single-share) path is unchanged.
- Under a deferred mode (`lazy: :multi` or `:batch`), `eval`/`evalsha` return a
  lazy value instead of sending immediately. Force the value (or use an eager
  client) to get the parsed reply. The explicit `Emb.multi { }` block API is
  unaffected.
