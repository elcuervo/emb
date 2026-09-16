## 1. Headless frame mode in `emb-top`

- [x] 1.1 Add a frame-streaming mode to `cmd/emb-top` that reuses the dashboard model (`poll` → `applyResult` → `View`) and writes one complete frame per poll interval to stdout, without a TTY or alternate screen; verify with `go test ./cmd/emb-top/` and by piping a run through `wc -l`.
- [x] 1.2 Force the color profile and a dark background in the mode so frames keep the dashboard's colors on a non-terminal stdout; verify a captured frame contains SGR color sequences and no bare monochrome output.
- [x] 1.3 Make the mode survive connection loss (keep polling, reconnect, rebase counters) instead of exiting like `-once`; verify with a node-restart test.
- [x] 1.4 Use the 300-poll window in the mode; verify the emitted frame draws the full five-minute history (charts/heatmap span the window) after enough polls.
- [x] 1.5 Cover the mode with tests (one frame per interval, frames complete, color present, reconnect) and verify `go test ./cmd/emb-top/ -count=1` passes.

## 2. Producer supervision in the bridge

- [x] 2.1 Have the bridge spawn exactly one producer child, restart it with backoff if it exits, and kill it on shutdown; verify with a test that uses a fake producer command and asserts a single restartable child.
- [x] 2.2 Parse each frame from the child and keep only the latest as an atomic snapshot for subscribers; verify with a unit test that a newer frame supersedes an older one.

## 3. Terminal frame to markup

- [x] 3.1 Implement the frame-to-markup conversion (HTML-escaped text plus styled spans) covering reset, basic/256/truecolor foreground and background, and bold/dim/italic; verify with table-driven tests for each SGR form.
- [x] 3.2 Verify a captured real frame converts with zero residual escape sequences and that a colored-background cell (the heatmap) survives as a background style.

## 4. Live endpoints

- [x] 4.1 Add `GET /api/stats` (SSE) that sends the current frame on connect and every new frame after; it MUST be GET-only and ignore any input; verify a test connects, receives the current frame, then receives the next one.
- [x] 4.2 Bound concurrent subscribers and answer beyond the bound with a legible state; verify a test that opens the cap and asserts the refusal without disrupting existing viewers.
- [x] 4.3 Emit an explicit unavailable state when the producer or node is down, rather than the last frame; verify a test with a stopped producer.
- [x] 4.4 Confirm the stream path does not touch `/api/exec`, the allowlist, or the spend bounds; verify by inspection and a test asserting no command is issued upstream.

## 5. The live view page

- [x] 5.1 Add an embedded `stats.html` served at `/stats` that names the sandbox's node, carries no control, and replaces its frame on each event; verify a bridge test serves the page and that it contains no form/input/button.
- [x] 5.2 Make the fixed 120×40 frame usable at narrow widths by scaling or horizontal scroll; verify at a phone viewport.
- [x] 5.3 Show a legible state when the stream cannot connect or ends, instead of an empty or stale frame; verify by stopping the producer and reloading.

## 6. Wiring and discovery

- [x] 6.1 Add the `/stats` link to the standalone terminal (`website/repl/index.html`); verify a bridge test asserts the reference and that following it serves the live view.
- [x] 6.2 Build and copy `cmd/emb-top` (`CGO_ENABLED=0`) in `website/repl/Dockerfile`; verify the sandbox image builds and contains the binary.
- [x] 6.3 Update the `justfile`/`AGENTS.md` bridge checks for the new page and stream; verify `just sandbox-test` and `just sandbox-build` pass.

## 7. End-to-end verification

- [x] 7.1 Run `just website-dev` and confirm `/stats` advances with color, that a late viewer sees the current frame immediately, and that the page issues no request that can change the node.
- [x] 7.2 Run `openspec validate live-emb-top-stats --strict` and the bridge tests, and confirm both pass.
