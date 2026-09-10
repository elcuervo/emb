# emb-top: live node dashboard

## Why

A running `emb` node already exposes everything needed to understand its health and throughput (`EMB.STATS`, `EMB.MODELS`, `EMB.INFO <model>`), but visualizing it requires piecing together raw RESP replies or running `redis-benchmark` against it. Operators and developers have no clean, real-time view of request throughput, per-model behavior, cache effectiveness, or CPU/memory pressure — while debugging a "stuck" node, watching a benchmark, or validating a deployment.

## What Changes

- Add `cmd/emb-top`: a Bubble Tea TUI that connects to a running `emb` node and renders live throughput dashboards using `ntcharts` (streaming line charts, sparklines, themed borders/labels).
  - Aggregate `req/s` and `tok/s` streaming charts from `EMB.STATS` counter deltas.
  - Per-model panel driven by `EMB.MODELS` + `EMB.INFO <model>`: per-model req/s, tok/s, err/s, avg latency, pooling/batching/quantization metadata.
  - Resource + cache row: cache hit ratio, cache rates, RSS mem, CPU %, connections, active requests, goroutines, uptime.
  - Eye candy + debuggability: braille/line chart styles, per-series colors, autoscaling, error-flash, pause/resume/reset keys, connection-loss banner with auto-reconnect, `AUTH` + TLS support.
- Ship `emb-top` wherever `emb` ships:
  - Docker image gains `emb-top` at `/usr/local/bin` (static, CGo-free build).
  - `emb-server` gem gains a per-platform `emb-top` binary (`lib/emb-server/emb-top-binary-*`) and a `bin/emb-top` wrapper (no onnxruntime needed).
  - `just build` / `just validate-gems` and the release workflow produce and publish it.
- Add a minimal RESP2 polling client (dial, pipeline `EMB.MODELS`/`EMB.INFO`/`EMB.STATS`, `AUTH`, TLS) — no new server-side commands or server code changes.
- **New `MONITOR` server command**: a bounded, seq-numbered ring buffer of completed-request events (model, text count, latency µs, error, unix-µs timestamp). Clients fetch incrementally (`MONITOR <afterSeq> [limit]`) on the same poll cycle. Feeds per-request latency percentiles (p50/p95/p99) and exact per-model visibility to the dashboard — no text payloads, best-effort buffer (bounded at 8192 events).
- Dashboard v2: model-activity **heatmap** (ntcharts heatmap: models × time, color = req/s), **latency percentile** stream chart, per-model p50/p95/p99, live event ticker, richer theme/layout.

## Capabilities

### New Capabilities

- `emb-top-dashboard`: the `emb-top` tool — connection/polling contract, throughput + per-model + resource/cache metrics, TUI interactions, and distribution in the Docker image and `emb-server` gem. New `specs/emb-top-dashboard/spec.md`.
- `emb-cmds`: new `MONITOR` command on the server (delta spec).

### Modified Capabilities

- `docker-build`: the image SHALL now also contain the `emb-top` binary (requirement change, delta spec).
- `emb-server-distribution`: the gem SHALL now also distribute the `emb-top` binary per platform with a `bin/emb-top` wrapper (requirement change, delta spec).

## Impact

- **Code**: new `cmd/emb-top/` (+ small `internal/embtop/` package for polling/rate math, unit-testable without the TUI); no changes to `internal/server`, `internal/pipeline`, etc. Only server-side usage is reading existing commands.
- **Server**: `internal/server/monitor.go` (ring + seq), `MONITOR` handler wired into the mux + help text; `handleEMB`/`handleEMBMULTI` record completion events (latency measured around inference, cache hits included).
- **Tool**: `internal/embtop` gains monitor reply parsing + event window with percentile computation; sampler ingests events per poll.
- **Go deps**: add `github.com/NimbleMarkets/ntcharts` (v1 branch), `charmbracelet/bubbletea` v1, `charmbracelet/lipgloss` v1 and their transitive deps to `go.mod`. All pure Go — `emb-top` builds without CGo/onnxruntime.
- **Env/deps**: `test-two-models.yaml` switches `minilm` to `model_repo: Xenova/all-MiniLM-L6-v2` so `just all` (which starts a server from that config) works from a fresh checkout without pre-downloaded ONNX files; verify everything builds/tests under `nix develop`.
- **Distribution**: `Dockerfile` (new stage or same builder stage, static build + `COPY`), `gems/emb-server` (`bin/emb-top` wrapper, gemspec `files`/`executables`), `justfile` (`build`, `validate-gems`), release workflow (build/attach per-platform `emb-top` for gems + image).
- **Docs**: README section on running/demoing the dashboard; BREAKING changes: none (new tool only).