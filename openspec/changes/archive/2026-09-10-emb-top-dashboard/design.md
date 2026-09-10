## Context

See proposal.md (motivation) and the delta specs (requirements). Existing state relevant to the approach:

- The server already exposes aggregate metrics via `EMB.MODELS`, `EMB.INFO <model>`, and `EMB.STATS` — all RESP2, all counters cumulative. These cannot give per-request latency distributions, so this change adds one server-side command (`MONITOR`): a bounded, seq-numbered ring of completed-request events.
- `EMB.INFO <model>` returns structured per-model stats (requests, tokens, errors, avg_latency_us, pooling, batching, quantization, model_bytes, per-model cache), so per-model data is parsed structurally — the `per_model` string in `EMB.STATS` is deliberately **not** used.
- Existing client-side Go tools (`cmd/emb-verify*`) use raw `net.Dial` + hand-rolled RESP; `go.mod` stays lean (no go-redis anywhere).
- Distribution: `Dockerfile` is multi-stage (CGo build → slim runtime); `gems/emb-server` ships per-platform binaries as `lib/emb-server/emb-binary-<platform>` with a thin `bin/emb` Ruby wrapper; `just validate-gems` copies `bin/emb` into the gem dir before building.
- Repo go version: 1.25. Repo culture: timing-sensitive tests must be deterministic (no wall-clock sleeps).

## Goals / Non-Goals

**Goals:**
- A single, small, debuggable TUI (`emb-top`) and a reusable, unit-testable polling/sampling core.
- Zero changes to the server binary or its protocol.
- Same distribution paths as the server: Docker image + `emb-server` gem (per-platform).
- A headless `-once` mode that makes the tool scriptable and the metrics testable in CI.

**Non-Goals:**
- No server-side metrics additions (no windowed percentiles, no new commands). `avg_latency_us` is shown as the lifetime average the server reports.
- No benchmark driving: `emb-top` is an observer. Load generation stays with `redis-benchmark`/`emb-verify-performance`.
- No standalone-release tarball changes in this change (follow-up if desired).

## Decisions

### 1. ntcharts v1 branch (classic bubbletea), pinned — not v2
The v2 branch targets Bubble Tea v2 (`charm.land/bubbletea/v2`) and currently **requires a `replace` to a neomantra fork of bubbletea "awaiting upstream merges"** — a build wrinkle that would land in `emb`'s go.mod. The v1 branch (`github.com/NimbleMarkets/ntcharts`) uses stable bubbletea v1.2.2 / lipgloss v1.0.0, go 1.22 (works with emb's 1.25), and has the same chart families we need (`linechart/streamlinechart`, `sparkline`, `barchart`). Pin a released v1 tag.
*Alternative considered:* v2 + neomantra replace (rejected: fragile, fork in the dependency graph for no chart benefit).

### 2. Layout: `cmd/emb-top/` (thin app) + `internal/embtop/` (client, sampler, headless output)
The existing `cmd/*` tools are single-file mains, but a TUI plus a RESP client plus rate math in one file would be ~1000 lines and untestable. `internal/embtop` mirrors the repo's `internal/*` layout:
- `internal/embtop/resp.go` — minimal RESP2 client: dial (plain/TLS), `AUTH`, pipelined `EMB.MODELS`/`EMB.INFO`/`EMB.STATS`, RESP2 reader/writer. Follows the `cmd/emb-verify` precedent (raw `net.Dial`, no go-redis).
- `internal/embtop/stats.go` — cumulative-counter sampler: per-poll deltas → rates, ring buffer (default 120s window), counter rebasing on reconnect, cache-hit-ratio, CPU% derivation.
- `internal/embtop/once.go` — headless mode: same sampler, prints one machine-readable line per poll, exit code 0/1.
- `cmd/emb-top/main.go` — bubbletea app: layout, ticker, keybindings, theme.
`internal/embtop` imports nothing from `internal/server`, so the tool builds with `CGO_ENABLED=0` (no onnxruntime/tokenizers).

### 3. Poll cycle: one pipelined round trip per tick
`EMB.MODELS` → `EMB.INFO <m>` per loaded model → `EMB.STATS`. One write, one read. `EMB.MODELS` each tick also discovers models loaded after startup. Default interval 1s, configurable (`-interval`). First poll seeds the previous-counter snapshot only (rates read 0 until the second poll).

### 4. Rates are deltas, rebased on reconnect
req/s = Δtotal_requests / Δt; tok/s, err/s, per-model rates likewise from `EMB.INFO` counters; CPU% = (Δcpu_user + Δcpu_sys) / Δt as percent-of-one-core; cache hit ratio = hits/(hits+misses). On reconnect after connection loss, the previous-counter snapshot is discarded so the first post-reconnect rates read 0 rather than a huge spur.

### 5. Layout & eye candy (design willing, "cool to see, easy to debug")
```
┌─ emb-top · node:16379 ────────────────────────────────┐  ← lipgloss header:
│ emb v0.2.4 · uptime 3h12m · models 2 · 1s poll (alt p) │    node addr, version, uptime
├──────────────────────────────┬────────────────────────┤
│ req/s    ▁▂▃▅▇ braille stream │ tok/s   ▁▂▃▅▇ stream  │  ← streamlinecharts,
│ (streamlinechart)            │ (streamlinechart)      │    autoscale, per-series colors
├──────────────────────────────┴────────────────────────┤
│ minilm     ▁▂▃▅▇  12.3 r/s   4.5k t/s   avg 456µs  err│  ← per-model rows:
│ bge-large  ▁▂▃▅▇   3.1 r/s   8.2k t/s   avg 1.2ms  err│    braille sparkline + text;
│ batching 10/512 · quant int8 · dim 384                 │    j/k scroll; errors flash red
├────────────────────────────────────────────────────────┤
│ cache hit ▇▇▂▇ 98.2% │ rss 512MB ▇▂ │ cpu 42% ▇▂ │ │  ← barchart gauges + sparklines
└────────────────────────────────────────────────────────┘
```
- Charts: `streamlinechart` for the two throughput streams (canvas width ≈ seconds at 1s poll); `sparkline` per model; `barchart` (horizontal) for the gauge row.
- Theme: lipgloss borders around panels, dim axis/labels, distinct line colors (req/s blue, tok/s green, per-model palette), red emphasis on any error rate > 0 (flashes while rising).
- Keyboard: `q` quit, `p`/space pause, `r` reset window, `j`/`k`/arrows scroll model rows, `?` help overlay.
- Reconnect: on poll failure show a connection-lost banner with last-good time; keep ticking; reconnect automatically.

### 6. Headless mode: `emb-top -once -samples N -interval 1s [-addr…]`
Prints `key=value` lines (aggregate + one section per model) to stdout, exits 0; exits non-zero if the node is unreachable. This is what makes the spec scenarios testable in CI and gives scripts the same numbers the TUI shows (pairs with `redis-benchmark`/`just bench-*` runs).

### 7. Distribution
- **Dockerfile**: in the builder stage, `CGO_ENABLED=0 go build -o /emb-top ./cmd/emb-top` (reuses `go mod download`); runtime stage `COPY --from=builder /emb-top /usr/local/bin/emb-top`. `ldflags` version like `emb`.
- **emb-server gem**: new `bin/emb-top` wrapper (platform switch → `lib/emb-server/emb-top-binary-<platform>`, **no** onnxruntime dependency); gemspec `files` add `emb-top-binary-*`, `executables` add `emb-top`. `just build` produces `bin/emb` *and* `bin/emb-top`; `just validate-gems` copies `bin/emb-top` into the gem dir (mirrors existing `emb-binary` step); release workflow builds `emb-top` per platform alongside `emb` for the gem builds and the image.

### 9. MONITOR: bounded ring + seq, fetched on the poll cycle
Per-request events are appended to a fixed ring (`8192`, seq-numbered) by `handleEMB`/`handleEMBMULTI` completion paths (latency = wall clock around inference, cache hits included, error flag set on failure, **no text payloads**). The tools fetches `MONITOR <afterSeq> [limit]` pipelined with the existing poll — one extra reply per tick. On a buffer gap (ring overwritten mid-poll), STATS/INFO deltas remain the rate source of truth and the monitor is rebased; no tool error.
*Alternatives considered:* Redis-style long-lived streaming MONITOR (rejected for v1: connection lifecycle + auth/TLS complexity for marginal benefit over a 1s incremental poll); windowed histogram in the server (rejected: more server state; per-request events keep the server stateless and the *tool* decides windows — p50/p95/p99 over the last 60s or 2048 events).

### 10. Dashboard v2: heatmap + latency percentiles + event ticker
Sections, top to bottom: header; **model-activity heatmap** (ntcharts `heatmap`, rows = models, columns = polls, color scale = req/s); req/s + **p95 latency** stream charts; per-model rows (sparkline + req/s, tok/s, **p50/p95/p99**, err) sorted by req/s; gauge row; **event ticker** (last request's model · texts · latency); footer. Latency percentiles computed from the monitor event window on each poll (sort ≤ 2048 most-recent latencies).

### 11. just all / nix develop correctness
`test-two-models.yaml` switches `minilm` to `model_repo: Xenova/all-MiniLM-L6-v2` so a fresh checkout's `just all` can start the server without pre-staged `./models` ONNX files (bge already downloads by repo). Full verification runs inside `nix develop` (`just build/lint/test/validate-gems`), plus the Ruby suite via `just all`.
### 12. Testing
- Unit: RESP decoding and counter→rate math in `internal/embtop` (deterministic fakes, no sleeps) — consistent with the repo's timing-sensitive-test rules.
- Server: `internal/server/monitor_test.go` — ring wrap/seq semantics + handler RESP shape against a real in-process server.
- Integration: start a real server, run `emb-top -once -samples 3`, assert line format and that rates are ≥ 0 and consistent with counters. Join `just test` / `just all`.

## Risks / Trade-offs

- **Terminal compatibility**: braille/color rendering varies (Kitty, Ghostty, iTerm, tmux, plain xterm). → Use ntcharts' rune styles with a sensible default (braille where supported, block fallback), declare minimum terminal (UTF-8, truecolor-optional) in README, keep colors cosmetic-only (info never conveyed solely by color except as an accent on already-visible error numbers).
- **v1 is the legacy branch**: future ntcharts work lands on v2. → Pin a released v1 tag; re-evaluate v2 after the bubbletea fork merge. Low blast radius: the TUI is isolated in `cmd/emb-top`.
- **go.mod growth**: bubbletea/lipgloss/ntcharts + transitive deps enter emb's go.mod. → Pure Go, build-tagged to the tool; `go vet ./...`/`go test ./...` cost is small; no CGo pollution, no impact on server binary.
- **Counter snapshots vs. server restarts**: counters reset on restart → clamp negative deltas to 0 and rebase (Decision 4).
- **Lifetime `avg_latency_us`** can look flat. → The dashboard shows windowed p50/p95/p99 from `MONITOR` events; the cumulative average remains as the fallback when no events are available.

## Migration Plan

Additive change: new tool + one new server command (`MONITOR`). Deploy path: merge → `just build`/`just test`/`just validate-gems` locally → release workflow publishes the updated image (now containing `emb-top`) and per-platform gems (now containing `bin/emb-top`). Rollback: previous image/gem versions remain valid; older `emb-top` binaries simply don't use `MONITOR` (the client degrades gracefully to rates + cumulative latency).

## Open Questions

- Multi-node/cluster aggregation (multiple `-addr` targets) — deliberately single-node for now; the monitor + heatmap lay the groundwork.
- Whether `MONITOR` should later expose truncated request texts (Redis MONITOR-style) — privacy/weight tradeoff, follow-up.