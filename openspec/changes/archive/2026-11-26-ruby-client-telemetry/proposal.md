# Proposal

## Why

The server already exports per-request latency and its own dispatch/admission/run
phases over OTLP, but the Ruby client exports nothing. An application can therefore
see what the server spent, and nothing about what its own calls paid — the
`lazy: :batch` coalescing queue, per-call latency, which instance answered, or
whether a batch's shares actually ran in parallel. Without those signals a
dashboard can show a server-side average but cannot attribute it to a request, a
model, or a batching mode.

## What Changes

- Make telemetry optional: no runtime dependency on OpenTelemetry. The API is
  required on first use, and without the gems every signal path returns
  immediately. With them installed, the API's default providers are no-ops until
  the app configures an SDK. No new gem option: `OTEL_SDK_DISABLED=true` remains
  the off switch.
- Spans: `emb.embed` per logical call — eager: the command; deferred: one per
  unique deferred item, starting at first deferral and ending at resolution;
  `emb.batch` per force per client; `emb.share` per command actually sent, with
  `model` and the instance that served it. Shares are children of their batch, so
  a trace flame graph shows real overlap.
- Metrics under `emb.client.*`: `embed.duration`, `queue.duration` (deferral cost),
  `batch.duration`, `batch.shares`, `batch.parallelism` (sum of share durations
  divided by batch wall time), `share.duration`, and `shares.in_flight`. Custom
  histogram boundaries from 1 ms to 30 s; SDK defaults would collapse embedding
  latency into a single bucket.
- A deferred item that never reaches a command (a serial-path batch failure) is
  recorded as not dispatched rather than as a fast inference.
- Document the signal map in `gems/emb/TELEMETRY.md` (linked from the README) and the CHANGELOG, including
  that the gem does not measure server-side phases and that client and server
  signals are joined on `model`.

### Non-goals

- No server, wire, or protocol change. `MONITOR` is not polled from the client:
  the server already exports the same phases over OTLP, and a client-side poller
  would duplicate that signal and need its own gap-handling.
- No per-inference compute time from the client. A batched command answers many
  texts at once; the client cannot split that reply's latency per text, and it
  will not claim to.
- No spans around `Emb.multi` blocks or `VALUES` decoding beyond the existing
  call sites.
- No sampling, exporter, or SDK setup in the gem: the application owns its
  `TracerProvider` and `MeterProvider`.

## Capabilities

### New Capabilities

- `ruby-client-telemetry`: OpenTelemetry spans and metrics emitted by the `emb`
  Ruby gem, their names, attributes, and what each one measures — including the
  batch shape and observed parallelism under `lazy: :batch`.

### Modified Capabilities

None. Existing client behavior — batching, coalescing, failure handling, and
configuration — is unchanged; this change adds an observational layer.

## Impact

- `gems/emb/lib/emb/telemetry.rb` (new), `batch.rb`, `batch_dispatch.rb`,
  `proxy.rb`, `commands.rb`, `batch_scope.rb`.
- `gems/emb/Gemfile`: OpenTelemetry SDK/exporter gems as development dependencies
  only; the gemspec gains no runtime dependency.
- `gems/emb/README.md`, `gems/emb/TELEMETRY.md`, `gems/emb/CHANGELOG.md`.
- New specs covering emitted spans/metrics against an in-memory OTel SDK test
  exporter, including the assertion that a `:batch` force produces overlapping
  `emb.share` spans.
- No server changes, no protocol changes, no breaking client API changes.
