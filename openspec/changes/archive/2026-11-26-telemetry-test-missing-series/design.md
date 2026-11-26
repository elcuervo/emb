# Design

## Context

See proposal.md. `TestTelemetryEndToEnd` currently stops after two exports, which may both precede publication of the request delta. The exporter may omit `emb.requests` when the interval has no eligible datapoints.

## Goals / Non-Goals

**Goals:** Wait for a captured export with a positive `emb.requests` delta, treating an omitted metric as zero, or fail at the test's existing deadline.

**Non-Goals:** Change exporter output, telemetry semantics, or production code.

## Decisions

- Treat a missing `emb.requests` metric as a zero delta only in the scan.
- Keep listening for exports and inspect each batch until one contains a positive delta or the existing five-second deadline expires. Do not stop merely because two exports arrived; the observed request may not have reached an export yet.
- Use the required metric lookup for the selected positive-delta export. Do not change the exporter to emit empty or synthetic series; that would alter telemetry output to accommodate a test assumption.

## Risks / Trade-offs

- [Risk] The test could pass if no captured export carries the request delta only if its final assertion is weakened. -> Keep the positive-delta assertion unchanged and exercise it in the targeted test.
