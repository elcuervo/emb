# Telemetry

Optional OpenTelemetry spans and metrics for the `emb` client: what a call pays,
including the `lazy` coalescing queue, and whether a `:batch` scope's shares
actually ran in parallel.

The gem has no runtime dependency on OpenTelemetry and no telemetry setting of its
own. Install the gems and signals flow; `OTEL_SDK_DISABLED=true` stops them. All
other configuration is standard `OTEL_*`, owned by your application.

## Enable

```ruby
# Gemfile — spans
gem "opentelemetry-sdk"                    # brings opentelemetry-api
gem "opentelemetry-exporter-otlp"

# ...and metrics (brings opentelemetry-metrics-api)
gem "opentelemetry-metrics-sdk"
gem "opentelemetry-exporter-otlp-metrics"
```

```ruby
# config/initializers/emb_telemetry.rb — the app owns the SDK and the exporter.
require "opentelemetry/sdk"
require "opentelemetry-metrics-sdk"
require "opentelemetry/exporter/otlp"
require "opentelemetry/exporter/otlp_metrics"

OpenTelemetry::SDK.configure do |c|
  c.service_name = "my-app"
  c.add_span_processor(
    OpenTelemetry::SDK::Trace::Export::BatchSpanProcessor.new(
      OpenTelemetry::Exporter::OTLP::Exporter.new
    )
  )
end

OpenTelemetry.meter_provider.add_metric_reader(
  OpenTelemetry::SDK::Metrics::Export::PeriodicMetricReader.new(
    exporter: OpenTelemetry::Exporter::OTLP::Metrics::MetricsExporter.new,
    export_interval_millis: 15_000
  )
)

# Boundaries per unit: embedding latency is seconds, the share count is a count,
# and parallelism is a ratio. The metrics SDK does not apply instrument advisories
# yet, so apply the gem's declared boundaries yourself, before the first call.
view = OpenTelemetry::SDK::Metrics::Aggregation::ExplicitBucketHistogram
OpenTelemetry.meter_provider.add_view("emb.client.*", type: :histogram, unit: "s",
                                      aggregation: view.new(boundaries: Emb::Telemetry::BOUNDARIES))
OpenTelemetry.meter_provider.add_view("emb.client.*", type: :histogram, unit: "{share}",
                                      aggregation: view.new(boundaries: Emb::Telemetry::COUNT_BOUNDARIES))
OpenTelemetry.meter_provider.add_view("emb.client.*", type: :histogram, unit: "1",
                                      aggregation: view.new(boundaries: Emb::Telemetry::RATIO_BOUNDARIES))
```

Spans land under the current trace, so a lookup inside a controller or job shows
the client's contribution to that request.

## Spans

```
rack.request
└── emb.batch                  mode=batch shares=4 texts=48 workers=3
    ├── emb.share  model=minilm instance=emb-0.internal:6379
    ├── emb.share  model=bge     instance=emb-1.internal:6379
    └── emb.share  model=minilm instance=emb-0.internal:6379
```

| Span | One per | Attributes |
|---|---|---|
| `emb.embed` | logical call — eager: the command; deferred: the deduped item, from deferral to resolution | `emb.operation`, `emb.model`, `emb.format`, `emb.texts`, `emb.mode`, `emb.dispatched`, `emb.queue` |
| `emb.batch` | forced batch per client | `emb.mode`, `emb.shares`, `emb.texts`, `emb.workers` |
| `emb.share` | command actually sent | `emb.model`, `emb.texts`, `emb.instance` |

`emb.share` spans are children of their batch and carry real start/end times, so a
flame graph shows the overlap: that *is* the parallelism. `emb.embed` spans under a
deferred mode start when the call was deferred, so their duration is what the
caller waited. An item that resolves without a command — a failed batch's unsent
remainder — is marked `emb.dispatched=false` rather than reported as a fast
inference.

## Metrics

| Metric | Type | Attributes |
|---|---|---|
| `emb.client.embed.duration` | histogram (s) | model, mode, format |
| `emb.client.queue.duration` | histogram (s) | mode |
| `emb.client.batch.duration` | histogram (s) | mode |
| `emb.client.batch.shares` | histogram | mode |
| `emb.client.batch.parallelism` | histogram | mode |
| `emb.client.share.duration` | histogram (s) | model, instance |
| `emb.client.shares.in_flight` | up-down counter | — |

`emb.client.queue.duration` ends when the batch starts dispatching, so it measures
what coalescing held a call before any command was sent. Under `lazy: :multi`, a
call that then waits behind an earlier share shows that wait in its `emb.embed`
span, not in the queue metric.

`emb.client.batch.parallelism` is the sum of a batch's share durations divided by
its wall time: `1` means the shares ran one after another, `N` means `N` ran at
once. Under `lazy: :batch` it should track the number of models (capped by
`pool × url count`); if it sits near 1 while models are idle, the dispatch stopped
being concurrent.

## Datadog

Point the exporter at the Datadog Agent's OTLP receiver
(`OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318`) or straight at Datadog's OTLP
intake.

```
avg:emb.client.batch.parallelism{*}            # parallelism over a batch workload
p95:emb.client.queue.duration{*}               # what coalescing costs
p95:emb.client.embed.duration{*} by {emb.model} # latency your app pays
```

## Limits

The client measures the round trip, not the server's compute split. For
queueing-versus-compute use the server's own OTLP series (`emb.request.duration`,
`emb.inference.duration`, `emb.dispatch.wait`, `emb.sessions.busy`, joined on
`model`), and [MONITOR](../docs/commands.md) for per-request detail.
`emb.client.share.duration` includes network, serialization, and the server's own
queue.
