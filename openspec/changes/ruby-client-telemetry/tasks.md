# Tasks

## 1. Dependency and telemetry seam

- [x] 1.1 Keep telemetry optional: no runtime dependency in `gems/emb/emb.gemspec`, with `opentelemetry-sdk` and `opentelemetry-metrics-sdk` as development dependencies; verify `bundle install` resolves and `cd gems/emb && bundle exec ruby -e "require 'opentelemetry-api'"` succeeds
- [x] 1.2 Add `lib/emb/telemetry.rb` resolving one tracer and one meter per process from `OpenTelemetry.tracer_provider`/`meter_provider`, with span/histogram/up-down-counter helpers and the explicit 1 ms–30 s histogram boundaries from `design.md`; verify a spec asserts that with no SDK configured a call emits nothing, raises nothing, and changes no result
- [x] 1.3 Require the telemetry module from `lib/emb.rb` and verify `bundle exec rake` stays green with no SDK present

## 2. Eager call spans

- [x] 2.1 Emit one `emb.embed` span from `Proxy#[]` and `Proxy#image` with operation, model, format, text count, and mode attributes; verify a spec with an in-memory SDK exporter asserts one span per call and the attribute values
- [x] 2.2 Emit the same span from the eager `eval`/`evalsha` path in `Commands`; verify a spec asserts the span for a script call and that the reply is unchanged
- [x] 2.3 Keep span completion outside the rescued command path so a raising span processor cannot change results; verify a spec with a raising processor asserts the call still returns its value and sends only one command

## 3. Deferred clock, queue, and scope flushing

- [x] 3.1 Record the first deferral time per item in a per-thread map keyed by the batch item, without adding a field to the item tuple; verify a spec asserts duplicate deferrals of the same text keep the first timestamp and still produce one server text
- [x] 3.2 Delete the entry on resolution and record `emb.client.queue.duration` plus the queue attribute on the item's span; verify a spec asserts the queue covers deferral→dispatch and that no entry survives a successful resolve
- [x] 3.3 Flush leftover entries on `BatchScope.wrap` teardown and `clear_batch_pending!`; verify a spec asserts a deferred item that is never forced emits exactly one not-dispatched span at scope end and leaves the map empty

## 4. Batch and share spans, parallelism

- [x] 4.1 Emit one `emb.batch` span per forced batch per client with share, text, and worker counts; verify a spec asserts one span per force and its counts for a two-model `:batch` scope
- [x] 4.2 Emit `emb.share` spans from the dispatch path, parented by activating the context captured on the forcing thread, with model and instance attributes and the in-flight counter incremented/decremented in an `ensure`; verify a spec asserts the instance attribute is `host:port` and contains no credentials
- [x] 4.3 Record `emb.client.share.duration`, `emb.client.batch.duration`, and `emb.client.batch.parallelism` from the outcome timings; verify a spec asserts a two-share concurrent batch reports parallelism near 2 and a `:multi` batch near 1
- [x] 4.4 Extend the existing latch-based batch spec to assert the two `emb.share` spans overlap in time; verify that spec fails if dispatch is made serial

## 5. Failure reporting

- [x] 5.1 Mark a failed share's span with error status and the recorded exception, and mark its items' spans failed; verify a spec asserts error status on the share span and a raised `Emb::ServerError` at the call site
- [x] 5.2 Ensure a serial-path batch failure reports its unsent items as not dispatched and contributes no share duration to `emb.client.share.duration`; verify a spec asserts the not-dispatched marker and the absence of a share observation for those items

## 6. Documentation

- [x] 6.1 Document the signal map in `gems/emb/TELEMETRY.md` (linked from the README) — span tree, metric table, attributes, the application-side SDK/OTLP setup for Datadog, and that client signals join server signals on `model`; verify every command in the snippet runs as written
- [x] 6.2 Add a CHANGELOG entry under `Added` describing the new dependency, spans, and metrics, noting that no SDK means no behavior change
- [x] 6.3 Add a line to `docs/operations.md` pointing at the gem's client-side signals and stating they never replace the server's `dispatch`/`admit`/`run` attribution; verify the link resolves

## 7. Integration verification

- [x] 7.1 Run the Ruby suite against a real server (`./bin/emb -config test-two-models.yaml -listen 127.0.0.1:16379` inside `nix develop`, then `cd gems/emb && bundle exec rake`) and `bundle exec rubocop`; verify both pass
- [x] 7.2 Measure overhead with no SDK configured using the gem's existing bench harness (`cd gems/emb && bundle exec rake bench`) before and after the change; verify the difference is within run-to-run noise and record the numbers in the CHANGELOG or the change
- [x] 7.3 Run `openspec validate ruby-client-telemetry --strict` and verify it passes

## Workflow follow-up

- Archive the change after review.
- Verify the archived spec at `openspec/specs/ruby-client-telemetry/spec.md`.
