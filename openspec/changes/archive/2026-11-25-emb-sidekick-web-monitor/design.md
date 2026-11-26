# Design

## Context

See `proposal.md` for motivation and `specs/emb-web-monitor/spec.md` for observable behavior. `internal/embtop` already supplies RESP polling, DNS expansion, rate/history sampling, health/advice calculations, and authenticated/TLS connections; `cmd/emb-top` owns its TUI and some fleet lifecycle/rendering code. `website/repl/stats.go` demonstrates one headless frame producer and bounded HTTP fan-out, but its bridge also exposes a command endpoint and is deliberately tied to one sandbox node. Neither its command surface nor its terminal-frame page belongs in the operator app.

## Goals / Non-Goals

**Goals:**
- A single runnable, CGO-free HTTP binary plus embedded, same-origin browser assets. The deployed process makes no outbound connection other than configured EMB nodes and optional VPC DNS, and requires no services, registry, npm installation, CDN, or installed ORT library at runtime.
- One operational model for a node or fleet, with bounded sampling cost independent of browser count and an interactive browser-native presentation of actual measured data.
- A distinctive, readable operator surface that respects EMB's existing visual language while working on desktop, phone, keyboard, and assistive technology.

**Non-Goals:**
- No persistent storage, alerts, accounts, arbitrary browser-selected targets, proxying EMB commands, custom queries, or new EMB server protocol.
- No mandate to move the sandbox `/stats` or replace `emb-top`'s terminal interface.

## Decisions

### 1. Package one Go HTTP process, not a Ruby web process or a sandbox fork

Add `cmd/emb-sidekick` with `net/http`, embedded HTML/CSS/JS, and a focused monitoring service that imports `internal/embtop`. Build with `CGO_ENABLED=0` using the module's existing dependencies; require no new modules or frontend package manager. Ship the binary with existing release archives and a local build target, but keep it independently runnable without ONNX or Ruby. Do not mount the existing `website/repl` bridge: it admits sandbox commands, hardcodes one producer and one upstream, and is not an operator security boundary. Alternative of serving `emb-top -frames` via a new wrapper saves sampling code but locks the UI to a fixed 120×40 terminal grid and hides structured measurements; reject it for this web app.

### 2. Keep one measurement pipeline and one bounded in-memory window

Resolve configured addresses/DNS names with `internal/embtop` and reuse its RESP `Client`, `Sampler`, health and recommendation functions. Give each resolved node its own connection, event cursor, sampling state and deadline; a hung node cannot stall peers. Move only reusable fleet membership/aggregation logic currently coupled to `cmd/emb-top` into `internal/embtop` where needed, and adapt the existing CLI to that shared logic rather than copy it. Do not import Bubble Tea/TUI rendering into the HTTP service. Keep a five-minute rolling window at the default one-second cadence plus bounded MONITOR events; rebase counters on reconnect/restart. Preserve first-seen node/model identity. The sidekick samples once per node per tick, not once per viewer.

The worker publishes a complete, immutable snapshot per tick. `GET /api/snapshot` supplies a current state plus the retained window to late viewers; `GET /api/stream` supplies subsequent samples by SSE. Serialize once per published tick, fan out to bounded subscribers with a one-snapshot channel, and drop superseded updates for slow browsers; a fresh snapshot replaces them without a historical gap being misrepresented as continuous. Browser time-range selection (recent minute or full retained window) filters that same source; no new data query endpoint or persistence layer. Existing server counters provide rate denominators, and recent `MONITOR` events provide latency/phase evidence; older nodes lacking attribution produce unavailable, never zero-derived claims.

### 3. Make the browser a native operator instrument, not a terminal replay

The first view is conditional: one node opens a node detail; several open fleet overview with its aggregate and stable, individually selectable node rows. Node detail has a clear health/status band, request and token trends, p50/p95/p99 latency and cache/uncached split where measured, CPU/memory/connection pressure, a per-model table with activity and phase detail, and a bounded recent-event list. Fleet view shows membership, distribution/skew, health per node and aggregate trend; selecting a node never loses the fleet context. Use existing health/advice interpretation, not a new severity formula in JavaScript. Expose honest no-sample, idle, stale, auth failure, partial fleet, DNS error and recovery states. Display update age and sample count alongside charts.

Follow the established EMB paper/ink/dark/orange palette and editorial rules, but use the restrained dark instrument treatment appropriate to an operations tool: typography, column alignment, strong rules, and one orange live-data signal rather than glow, gradients, decorative animation, or generic SaaS cards. Render responsive charts with native SVG/HTML/CSS and small first-party JavaScript. Axes, units, time bounds, and legible numeric summaries make charts interpretable; tables remain readable at narrow widths by reflowing hierarchy rather than scaling down a terminal. Every interactive element works by keyboard, with visible focus and semantic names; status has non-color text, chart summaries have text alternatives, announcements are throttled, and `prefers-reduced-motion` removes transitions. No font or asset fetch to an external origin; system fonts or embedded assets only. Visual proof during implementation must inspect actual desktop/mobile pages with live node/fleet samples and stale/empty states, not just snapshots of markup.

### 4. The viewer cannot control a RESP connection

Only startup flags/environment configure monitored targets, credentials, TLS and bind address; never interpolate browser input into a dial target or RESP command. Reuse upstream TLS hostname verification for DNS-expanded peers. Read the upstream password from process environment rather than a CLI argument that appears in process listings; never include it in logs, JSON, markup, or errors. Escape user-controlled names in browser rendering using text nodes; no raw HTML from EMB. Serve only static assets and GET-only snapshot/stream/health endpoints with no CORS wildcard, no command proxy, no mutable API, no echo of upstream URLs containing credentials, and no automatic remote asset loads. Bound request headers, connected viewers, frame size and idle SSE clients; serve security headers appropriate to static same-origin assets.

The HTTP listener defaults to `127.0.0.1`; `-listen` explicitly permits a private VPC interface. The binary does not claim that VPC membership authenticates people. Document network ACL/VPN/private ingress as the operator's viewer-access boundary; absent such a boundary, bind loopback and do not publicly expose HTTP or send credentials to it. Upstream TLS protects RESP credentials on the wire; HTTP ingress protection is a separate deployment responsibility. Built-in logins/HTTPS would introduce a new configuration and credential lifecycle not requested, so they are not silently implied by this design.

## Risks / Trade-offs

- [In-memory history disappears on restart and MONITOR has a finite ring] → Label the available window and sample counts, show gaps and fresh starts, do not imply incident playback or full capture of high-volume events.
- [Shared sampling and CLI fleet semantics could diverge] → Extract only pure/common fleet lifecycle and use the same poller/health/advice functions; exercise parity for rates, connection states and advice on the same inputs.
- [Slow clients and many monitored nodes can exhaust process resources] → Bound subscriber count, queue depth, history/events and per-node deadlines; drop obsolete snapshots and expose freshness instead of accumulating unbounded work.
- [Operator HTTP data reveals topology and operational state] → Default to loopback, restrict ingress when binding privately, expose no upstream credentials or payloads, and document that private HTTP alone is not authentication.
- [No web rendering/chart dependency increases frontend work] → Prefer semantic HTML and restrained SVG for a small set of essential charts, with visual checks and accessible text rather than adding a framework or CDN.

## Migration Plan

1. Build `emb-sidekick` independently and add it to existing release archives; no existing binary or wire contract changes.
2. Document a loopback single-node invocation and a private-interface fleet invocation with upstream AUTH/TLS and network ingress restrictions.
3. Roll back by stopping the sidekick; EMB and `emb-top` remain unchanged and there is no state to migrate.
