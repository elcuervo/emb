# Proposal

## Why

Operators can already inspect EMB from a terminal with `emb-top`, but there is no standalone, browser-native HTTP monitor they can run inside a closed VPC. The sandbox's `/stats` is tied to its demonstration node and replays terminal frames rather than offering an operational view of a configured node or fleet.

## What Changes

- Add a standalone `emb-sidekick` HTTP application for live, read-only monitoring of one EMB node or a configured fleet, with an overview and node/model drill-downs in the same interface.
- Build a deliberate, responsive operator UI: live charts, health and pressure signals, recent-request detail, honest disconnected/stale states, keyboard navigation, and reduced-motion and screen-reader alternatives. Reuse the measurements and interpretation behind `emb-top`; do not turn the sandbox console into the operator product.
- Package the server and all browser assets together, with no CDN, remote fonts, hosted telemetry, external database, npm runtime, or other runtime service besides the monitored EMB nodes and optional VPC DNS. Add no new third-party build dependency; the application can run without internet access.
- Keep monitoring targets operator-configured, never supplied by browsers. Bind loopback by default; document explicit private-network binding and network access control. This is a read-only monitor, not an EMB administration or command proxy.
- Document how to build, run, connect to protected/TLS nodes, and safely expose the HTTP listener to operators inside a VPC. No existing EMB or `emb-top` command/reply contract changes.

## Capabilities

### New Capabilities

- `emb-web-monitor`: Self-contained operator-facing HTTP monitoring of a configured EMB node or fleet, including live presentation, offline operation, and safe access boundaries.

### Modified Capabilities

- None. `emb-top-dashboard`, `emb-top-cluster-view`, and `sandbox-live-stats` retain their current external behavior.

## Impact

- New Go command and embedded browser assets; shared use of the existing `internal/embtop` poller and, where required, its health/advice calculations currently owned by `cmd/emb-top`.
- Distribution/build and operations documentation for the sidekick, with focused monitoring and HTTP/browser checks. No server protocol changes or new external runtime/build dependencies.
