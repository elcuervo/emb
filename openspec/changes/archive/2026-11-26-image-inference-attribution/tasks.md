# Tasks

## 1. Capture image work phases

- [x] 1.1 Return preprocessing, image-session dispatch, admission, and inference-run durations from image work to each request; add deterministic tests for cache miss, full hit, partial hit, and multi-chunk attribution.
- [x] 1.2 Aggregate per-model image phase totals and session utilization without opening lazy image resources during reads; verify `EMB.INFO`/`EMB.STATS` snapshots against fake image sessions and unloaded models.

## 2. Publish and document attribution

- [x] 2.1 Extend `MONITOR` image event encoding with preprocessing and truthful phase availability; update RESP count/parser tests and verify image bytes remain absent from event data.
- [x] 2.2 Document the image timing fields and cache semantics in command/operations docs; verify documented field names match `MONITOR`, `EMB.INFO`, and `EMB.STATS` replies.
- [x] 2.3 Run focused server tests for `EMB.IMG`, `EMB.IMGMULTI`, cache behavior, and observability to verify phase totals remain monotonic under concurrent runs.
