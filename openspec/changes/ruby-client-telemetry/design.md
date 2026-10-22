# Design

## Context

See `proposal.md` — Why. What the implementation has to work within:

- **Call path.** `Proxy#[]`, `#image`, and `Commands#run_script_command` either send
  immediately (`send_command` → `ConnectionRouter` → `RoundRobinPool`) or, under a
  deferred `lazy` mode, return a `BatchLoader` whose batch key is the *item array*
  `[client, model, text, format]`. The force path is `BATCH_BLOCK` in `batch.rb`:
  group by client, pack by `batch_size`, then `dispatch_serial` (`:multi`) or
  `dispatch_parallel` (`:batch`).
- **batch-loader 2.0.6 semantics** (verified in `lib/batch_loader/executor_proxy.rb`):
  pending items live in a `Set`, loaded values in a `Hash`, both keyed by the item
  and compared with `eql?`/`hash`. Identical tuples therefore **collapse into one
  server item**. After the batch block, `__ensure_batched` calls
  `loader.call(item, default_value)` for every item the block left unresolved — so a
  deferred item can resolve without ever being sent.
- **Dispatch.** `dispatch_parallel` spawns `min(shares, worker_capacity)` real
  threads; `worker_capacity` is the sum of pool sizes. `RoundRobinPool#acquire` pops
  a distinct connection index from a `Queue` with no global lock, so up to `pool`
  commands per instance are genuinely in flight. Workers only *capture* outcomes;
  loaders are resolved on the forcing thread because batch-loader's executor is
  thread-local.
- **Existing timing.** `ConnectionRouter#perform_command` already has a
  `Emb.debug?`-gated monotonic clock around each command — the seam, currently
  discarded.
- **Server-side signal.** The server already exports per-request latency and its
  `dispatch`/`admit`/`run` phases over OTLP (`internal/telemetry/metrics.go`).

## Goals / Non-Goals

**Goals:**
- Client-observed latency and the deferred-mode coalescing cost, attributed to
  model and batching mode.
- Batch shape and *observed* parallelism, readable as overlapping spans and as a
  single numeric series.
- Inert: no SDK, no cost, no behavior change; no gem configuration surface.

**Non-Goals:**
- Measuring how long the server spent per text, and correlating a specific client
  call to a specific `MONITOR` event.
- Instrumenting `Emb.multi` blocks, `Emb.stats`/`info`/`config`, or the eager-burst
  pipelining example.
- Shipping an exporter, SDK, sampler, or OTLP endpoint handling.

## Decisions

### Optional API, loaded on first use, no configuration

No runtime dependency on OpenTelemetry. `Telemetry.available?` requires the API on
the first signal path and caches the answer; without it every path returns
immediately (one branch per call). With it, the API's no-op providers absorb the
signals until the app configures an SDK, so no gem setting is needed either way.
`OTEL_SDK_DISABLED` is the standard off switch.

*Alternatives:* a hard `opentelemetry-api` dependency — rejected because it forces
an install (and four transitive gems) on every consumer of a lean client; a
`telemetry` boolean — rejected as a knob with one sane value. Load order is not a
problem: the API is required lazily at first use, and the API's proxy providers
delegate to an SDK configured afterwards. *Cost:* zero when absent; ~1.2 µs per
call when present without an SDK.

### Span layout: call spans are backdated, not opened at deferral

```
caller context
└── emb.batch                      (forcing thread, one per force per client)
    ├── emb.share  model=… instance=…   ┐ created inside workers, parented via the
    ├── emb.share  model=… instance=…   │ context captured on the forcing thread
    └── emb.share  model=… instance=…   ┘
emb.embed spans (one per unique deferred item) are children of the *caller's*
context, created at resolve time with `start_timestamp:` = first deferral.
```

Creating `emb.embed` at deferral and finishing it at resolve was rejected: an item
that is never forced would leak an unfinished span (and a non-zero in-flight gauge)
until process exit. Backdating means unforced items simply emit nothing.

*Constraint discovered:* the deferral clock cannot live in the item tuple — that is
the dedupe key, so any per-call value would make every call unique and duplicate
server inference. It lives in a per-thread `Hash` keyed by the item, `||=` so the
first deferral wins, deleted in `resolve_slice`.

### Deferral registry flushed with the batch scope

Entries left in that `Hash` are exactly the items that were deferred and never
sent. `BatchScope.wrap` and `clear_batch_pending!` flush it, emitting one
`emb.embed` span per leftover with `emb.dispatched` = false, ending at flush time.
Apps without the middleware never flush it (the same contract the batch scope
already has — they must clear it), so the registry is only trusted to report
not-dispatched items when a scope is cleared.

*Alternative:* `ObjectSpace::WeakKeyMap` (measured on Ruby 3.4.11) — entries are
tied to the exact key object, so it works only because batch-loader's `Set` pins the
first instance; explicit deletion is already required, so the weak map buys nothing
and hides the coupling.

### `emb.share` spans are created on the forcing thread

A worker thread has an empty `Context.current`; spans created there would be
parentless. Capture `OpenTelemetry::Context.current` before spawning, activate it in
the worker around `dispatch_slice`, and let the worker return its timing. This keeps
resolution and span finishing single-threaded.

### Parallelism is computed, not inferred

`emb.client.batch.parallelism` = Σ share duration / batch wall duration, computed
once per force from the outcome timings, so a dashboard needs one series. The
denominator runs from first dispatch to last outcome; the in-flight up-down counter
is incremented/decremented inside an `ensure` so a raising share still decrements.

### Instance identity without credentials

`emb.share` needs the instance that served it. `ConnectionRouter#call` picks and may
re-pick the index internally, so the chosen index is published to a thread-local for
telemetry to read immediately after the command. The attribute is the URL's
`host:port` — never the URL string, which can carry a password in its userinfo.

### Error and not-dispatched status

A share failure marks `emb.share` with error status plus the recorded exception;
the `emb.embed` spans of that share's items are marked failed. Items that resolve to
`default_value`/`FailedShare` without a sent command are marked not dispatched (see
above) so a dashboard never averages them as fast inferences.

### Explicit histogram boundaries are declared, then applied by the app

`[0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30]`
seconds, declared on each instrument through its advisory parameters.

Measured, not assumed: `opentelemetry-metrics-sdk` 0.22 accepts instrument
advisories and does **not** apply them (`meter.rb`: "advisory parameters are
accepted but not yet applied to stream configuration"), and its default boundaries
are `[0, 5, 10, 25, …]` — milliseconds — so every embedding would land in the first
bucket. The gem therefore declares the boundaries and documents the one-line
`meter_provider.add_view('emb.client.*', aggregation:
ExplicitBucketHistogram.new(boundaries: Emb::Telemetry::BOUNDARIES))` an
application adds before its first call. Verified against the SDK: with the view,
exported datapoints carry those bounds.

### Timing sources, per metric

| Observation | Starts | Ends |
|---|---|---|
| queue | first deferral of the item | the batch's dispatch start |
| share | immediately before the command | reply received (decode excluded) |
| batch | first share dispatch | last outcome |
| embed (deferred) | first deferral | after decode of the item's value |
| embed (eager) | before `send_command` | after decode |

The queue is stamped once per slice on the forcing thread, never inside a worker:
the deferral registry is thread-local, so a worker could not read it. A serial
(`:multi`) call that then waits behind an earlier share keeps that wait inside its
`emb.embed` span rather than the queue metric.

## Risks / Trade-offs

- **Span volume in eager mode** (one per call) → capped by the app's sampler; the gem
  deliberately does not sample.
- **Hot-path allocation** when no SDK is configured → measured at **1.1 µs per
  call** against a fake client, i.e. ~0.1 % of a 1.3 ms single-text round trip and
  ~0.2 % of a batched per-embed. The eager path deliberately does not attach its
  span to the context (no child spans), which saved 0.5 µs per call; only a batch
  pays that. The end-to-end bench could not attribute any difference: its
  uninstrumented `pipelined` scenario moved as much as the instrumented ones
  between runs.
- **The metrics API is unstable (0.x)** → the global meter accessors are a
  separate require (`opentelemetry/metrics/global`), guarded with `rescue
  LoadError`; the gemspec pins `~> 0.10.0` so a minor release cannot silently
  change the API under a running app.
- **Backdated spans** depend on the SDK honoring `start_timestamp`; the no-op path
  ignores it, which is correct but means tests need a real SDK to assert durations.
- **`emb.embed` spans are per unique item, not per Ruby call** → callers that ask the
  same text twice see one span. Truthful (one inference) but documented.
- **Thread-local deferral registry** → unbounded only for apps that defer and never
  clear a scope; same pre-existing contract as batch-loader's own scope.
- **Repeated `emb.share` spans for retried commands** → a re-send shows as one span,
  since retries happen inside `ConnectionRouter`; attempts stay visible on
  `Emb::ServerError` as today.

## Migration Plan

Gem release only; no data or wire migration. Rollback is pinning the previous gem
version. Emitting no signals without an SDK means no behavior change for existing
users on upgrade.
