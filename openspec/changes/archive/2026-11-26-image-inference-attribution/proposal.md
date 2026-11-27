# Proposal

## Why

Image requests appear in `MONITOR`, but the server currently marks them unattributed and reports no dispatch, admission, or run phases. Operators therefore cannot distinguish preprocessing cost, image-session contention, inference time, and cache hits when diagnosing image workloads.

## What Changes

- Attribute image request phases at the image pipeline: preprocessing, session dispatch, CPU admission wait, inference run, and cache-served work.
- Expose image-session and phase information through the existing monitoring and server observability surfaces without recording image bytes.
- Preserve bounded, non-blocking metrics collection and report unavailable phases as unavailable, not as zero-cost work.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `server-stats-observability` — define accurate per-request and per-model image-path phase attribution.

## Impact

- Go server image preprocessing, image session dispatch/inference, `MONITOR`, `EMB.STATS`/`EMB.INFO`, telemetry, tests, and command/operations documentation. No new dependency or change to image-byte privacy boundaries.
