# Design

## Context

See `proposal.md` and `specs/server-stats-observability/spec.md`. `EMB.IMG` currently aggregates cache hits in `MonitorEvent` but image tensor preprocessing and `ImageResources.Embed` run outside the text pool's attribution path. `EMB.INFO`/`EMB.STATS` already build a shared snapshot for per-model resource reporting.

## Goals / Non-Goals

**Goals:** carry actual image work phases from preprocessing and session execution to request and model observability; retain low-allocation reads and avoid opening lazy image sessions just to inspect metrics.

**Non-Goals:** record image bytes, change image batching/preprocessing, or change text/script attribution semantics.

## Decisions

- Capture durations at the image work sites and return them with each image chunk's result; aggregate them through `embedImages` and `EMB.IMGMULTI` pair processing. Do not infer phase timings by subtracting from whole-request latency.
- Add an image preprocessing duration alongside the established dispatch, admission, and run fields. Mark attribution available only when that phase data exists; cache-only work reports cached item counts without fabricated inference durations.
- Keep image totals separate from text and script counters. Publish them through the existing model snapshot path, using image resource stats only when already materialized; metric reads must not call the image resource initializer.
- Extend the structured `MONITOR` representation and its RESP serialization additively. Keep command array counts and monitoring parsers aligned; continue excluding payload bytes.

## Risks / Trade-offs

- [Risk] Instrumentation can distort small image runs → use monotonic timestamps at phase boundaries and aggregate once per chunk/request rather than per tensor element.
- [Risk] Concurrent chunks can misattribute timings if counters are read from shared sessions → return per-run timing values to the caller that paid them, then aggregate those values.
- [Risk] Added observability fields can break positional RESP readers → add fields compatibly and update declared counts, client parsing, and contract tests together.

## Migration Plan

Additive response fields require no client migration. Deploy the server with the existing image path; rollback by reverting the server release. Existing image inference and cache contents remain unchanged.
