# Changelog

All notable changes to the `emb` gem are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [0.4.3.pre4] - unreleased

### Changed

- `lazy: :batch` now splits a resolving scope into one plain `EMB <model> <text>...`
  share per model and dispatches every share concurrently; it never sends
  `EMB.MULTI`. A two-model scope runs concurrently even when it fits in a single
  `batch_size` chunk. `lazy: :multi` still coalesces into serial `EMB`/`EMB.MULTI`.

### Fixed

- A deferred call and each `client.multi` entry copy their text (and image bytes) when
  the call is made, so mutating or re-tagging the caller's String afterwards can no
  longer change the sent `EMB`/`EMB.MULTI`/`EMB.IMGMULTI` payload.
- A failed share in a serial batch window no longer raises on a different client's
  loader or drops that client's queued work: the window's other groups dispatch on
  their own, and only the failed slice's items raise that share's `Emb::ServerError`.
  A single-client window where nothing was delivered still fails closed (raise, then `[]`); with several clients, each failed share keeps its own error.
- Deferral telemetry is keyed by the pending item's identity instead of its content
  hash, so a payload mutation can no longer strand an entry: its span closes and the
  scope's registry drains.
- Image coercion returns a fresh binary String instead of re-tagging the caller's
  String as `ASCII-8BIT` in place.
- A deferred single-image call decodes the one keyed map `EMB.IMG` replies with
  instead of raising `ArgumentError`; the share's image count, not its call
  count, selects the reply shape.
- `Emb.multi` blocks send the group's `OUTPUTS` on `EMB.IMGMULTI`, so `outputs:`
  narrows the projection instead of being dropped.
- A deferred `image(io)` reads the stream against the scope's remaining
  `max_image_batch_bytes` and raises `Emb::ImageBatchLimitError` while reading,
  instead of materializing an oversized payload first.

### Added

- Image calls defer under the configured lazy mode. `Emb[:model].image(bytes)`
  under `lazy: :multi`/`:batch` sends nothing until forced, then coalesces
  compatible calls (same model, format, and `outputs:` projection) into
  `EMB.IMG` shares — serial under `:multi`, concurrent under `:batch` — while
  keeping the eager single-image Hash / multi-image Array shape. Deferred bytes
  are captured at call time and bounded per scope by `batch_size` image payloads
  and the new `max_image_batch_bytes` (32 MiB default, global and per client);
  exceeding either raises `Emb::ImageBatchLimitError` before anything is queued.
- Image calls join the eager `Emb.multi` / `Client#multi` block exactly like
  text: `m[:model].image(bytes, *more, format:, outputs:)` accepts the same
  arguments as a single call and returns one output-name Hash per image in
  submission order, `nil` for a failed or truncated pair. Compatible calls
  (same format and `outputs:` projection) share one `EMB.IMGMULTI`; a block
  mixing text and images sends `EMB.MULTI` and `EMB.IMGMULTI` and reassembles
  results in submission order. `:values` decoding keeps the pair's `model` tag.
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
