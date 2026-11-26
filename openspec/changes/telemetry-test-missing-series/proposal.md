# Proposal

## Why

The race-detector CI run still fails in `TestTelemetryEndToEnd`: after tolerating exports without `emb.requests`, the test stops after only two exports, before either contains the request delta. Wait for an export with a positive delta until the existing timeout, and fail only if none arrives.

## What Changes

- Treat an absent `emb.requests` metric as zero for that export.
- Continue waiting for exports until one carries a positive request delta or the existing deadline expires; preserve the positive-delta assertion.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

None. This changes test handling only; exported telemetry behavior and its contract remain unchanged.

## Impact

- `internal/server/telemetry_test.go`
- No runtime code, API, dependencies, or telemetry specification changes.
