# Tasks

## 1. Shared monitoring model

- [x] 1.1 Reuse `internal/embtop` node parsing, RESP polling, sampler, health/advice and TLS verification in a TUI-independent monitoring core; verify an existing `emb-top -once` sample and the same input in the new core agree on rates, health and absent fields.
- [x] 1.2 Share only the fleet membership/aggregation behavior the HTTP app needs with `cmd/emb-top` instead of duplicating it; verify DNS join, responsive orphan retention, node recovery, stable identity and aggregate rebasing with deterministic tests for both consumers.
- [x] 1.3 Add independent per-node poll scheduling, bounded five-minute history and bounded recent events; verify a stalled peer cannot block another, resets cannot spike rates, and old data is evicted under sustained samples.

## 2. Self-contained HTTP service

- [x] 2.1 Add `cmd/emb-sidekick` with required operator-configured target(s), default loopback listener and a clear startup diagnostic for missing targets; verify launch against one local node and reject a no-target launch.
- [x] 2.2 Implement a latest-complete `GET /api/snapshot` and live `GET /api/stream` SSE from the single poller, with bounded subscriber slots and superseding updates; verify late join, two simultaneous viewers with one poll per node, slow-viewer recovery and stream reconnection.
- [x] 2.3 Keep the HTTP surface read-only and targets server-side, embed all assets, bound request/stream resources and emit same-origin security headers; verify POST/target injection cannot dial or command an EMB node and asset/page requests do not leave the sidekick origin.
- [x] 2.4 Wire upstream password from environment and TLS with DNS-name verification; verify authenticated and rejected credentials, certificate mismatch and no credential value in responses, assets or logs. Document loopback default, explicit VPC bind and ingress controls alongside the new command.

## 3. Operator web interface

- [x] 3.1 Create the browser-native, dark EMB operator shell and conditional first view (single node directly; fleet overview for multiple) with semantic navigation; verify real one-node and two-node sessions select the correct first screen and round-trip node drill-down.
- [x] 3.2 Build fleet hierarchy with aggregate timeline, health and pressure readouts, membership and stable per-node rows; verify a live, idle, degraded and disconnected node remain distinct in browser-visible state, with timestamps and no fabricated aggregate data.
- [x] 3.3 Build node/model detail with measured request/token/latency and cache/CPU/resource charts, model activity/phase evidence and recent events; verify on an EMB node under traffic that charts and numbers move together, missing attribution is labelled unavailable, and no request payload is displayed.
- [x] 3.4 Handle no samples, partial fleet, DNS failure, browser stream loss, node restart and empty event windows explicitly; verify transitions to stale/reconnecting and back to fresh without showing the last frame as current or inventing pre-restart history.
- [x] 3.5 Finish responsive and accessible interaction: real SVG axes/units plus textual summaries, keyboard traversal, contrast/focus, reduced motion and restrained live announcements; verify actual desktop and phone browser surfaces and keyboard/reduced-motion states on the running app. Document UI operation and in-memory five-minute retention beside the command guide.

## 4. Distribution and integration evidence

- [x] 4.1 Add `CGO_ENABLED=0` local build and include the standalone binary in existing release archives without a new Go or frontend dependency; verify the produced binary starts without ONNX/Ruby, its embedded assets load with internet blocked, and release package member checks include it.
- [x] 4.2 Exercise an end-to-end local node and two-node fleet through the HTTP sidekick, then stop/restart one node while browsers remain open; verify view recovery, freshness, aggregate consistency and unchanged `emb-top`/sandbox behavior with targeted Go checks and an observed browser smoke run.

## Workflow follow-up

- Archive the change after implementation and review; do not treat this planning artifact as authorization to implement.
