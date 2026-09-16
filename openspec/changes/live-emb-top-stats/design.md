## Context

See `proposal.md` for motivation. The state that shapes the approach:

- `cmd/emb-top/main.go` is a Bubble Tea TUI. Its `tuiModel.View()` returns a
  **complete frame** as a string: the whole dashboard, one row per line, color
  carried as inline SGR sequences. Cursor movement, alternate screen, and
  differential repainting live in Bubble Tea's renderer, not in `View()` — so a
  frame is directly consumable without a terminal emulator.
- `internal/embtop` is the reusable half: the RESP polling client and the
  `Sampler` that diffs cumulative counters into rates and percentiles. It is pure
  Go (`CGO_ENABLED=0`) and already has the `-once` headless consumer in
  `internal/embtop/once.go`.
- The sandbox bridge (`website/repl/`) is the one public surface: it serves `/`,
  the client module, and `/api/*`, and 404s every other path (`handleTerminal`).
  It embeds its pages with `//go:embed`. The server listens on loopback and never
  faces the browser.
- `github.com/charmbracelet/bubbletea`'s `init()` (`tea_init.go:21`) calls
  `lipgloss.HasDarkBackground()`, which waits out `termenv.OSCTimeout = 5s` when
  a TTY does not answer the OSC query. Measured with a non-TTY stdout the call
  returns immediately; in a pty it is the blank five seconds the recording rig
  documents. This is a property of the *importing process*, which matters for
  where the dashboard is rendered.
- `website/repl/allowlist.go` refuses `MONITOR` to visitors on the stated ground
  that "request events would describe other visitors' traffic". A server-side
  producer is a different party, but the same data, and no frame carries request
  text.

## Goals / Non-Goals

**Goals:**

- The live view is the dashboard's own output, in color, not a second rendering.
- One poll per interval for the whole sandbox, however many viewers.
- Read-only at the surface: no request the page can make reaches the command
  surface.
- No pty, no terminal emulator, no new dependency.
- A late viewer sees the current dashboard, never a partial paint.

**Non-Goals:**

- No interactive control of the producer (pause, scroll, reset).
- No second dashboard implementation and no re-derivation of metrics.
- No change to the allowlist, the spend bounds, the presets, or the server.
- No recording: the view is live only, and nothing is persisted.

## Decisions

### Frames are the TUI's own `View()`, and each frame is complete

The unit on the wire is the dashboard's rendered frame. This is what makes the
feature cheap: there is no HTML dashboard to write, no metric to re-derive, and
no way for the live view to disagree with the terminal. It also removes the
hardest problem in live terminal streaming — reconstructing screen state for a
late joiner — because a complete frame *is* the state. No virtual terminal is
needed, and no consumer has to replay history.

*Alternatives rejected:* a purpose-built HTML dashboard (a second implementation
of the same panels, and the thing the user explicitly did not ask for); streaming
raw pty bytes and replaying them (Bubble Tea repaints differentially, so a late
joiner would need a VT emulator to reconstruct the screen).

### The dashboard is rendered by an `emb-top` child, not inside the bridge

`emb-top` gains a headless frame-streaming mode; the bridge spawns one and reads
its frames. The alternative — moving the model into a package the bridge imports —
pulls `ntcharts` (and therefore `bubbletea`) into the bridge, whose `init()` runs
the `termenv` background query. In production (non-TTY) that returns at once, but
any developer running the bridge from a terminal would pay five seconds at
startup, and the web process would carry the TUI's whole rendering dependency
graph for no reason. A child process also keeps the mode exactly where the model
already lives, so the change to `cmd/emb-top` is additive rather than a move.

*Alternatives rejected:* run the existing TUI in a pty and snapshot it with `tmux
capture-pane` (a new system dependency in the sandbox image, and tmux quantizes
the palette unless RGB pass-through is configured); one pty and one `emb-top`
per viewer (the node's polling cost grows with the audience, and a terminal
emulator has to ship to the browser).

### The bridge turns terminal cells into markup; it does not emulate a terminal

A frame contains only SGR sequences, text, and newlines. The bridge converts
those to markup — a `<pre>` of escaped text with styled spans — and replaces the
previous frame. Because the frames are complete, the page never has to interpret
cursor movement or maintain screen state. This is the only genuinely new logic,
and it is small and testable in Go.

The producer forces `termenv.TrueColor` and a fixed dark background before
rendering, so the frame does not depend on the pipe's detected profile and the
colors match the terminal's.

### One producer, one broadcast, bounded subscribers

The producer polls the node once per interval; the bridge fans each frame out to
every subscriber. A viewer costs a socket and a frame copy, not a poll. The
subscription count is bounded, and the endpoint refuses beyond the bound rather
than admitting unbounded sockets; the frames are small but the endpoint is
public.

### Read-only by construction

The stream endpoint is `GET`-only and ignores any input; the producer talks to
`emb` on loopback and holds the only `MONITOR` cipher on the sandbox; the page is
static and has no control that submits anything. There is deliberately no path
from the page to `/api/exec`, and the producer is not chargeable against the
visitor spend bounds, because it spends no inference work.

### Size, window, and discovery

A frame is rendered at a fixed 120×40 (the recording's 120×32 plus headroom for
the per-model panel); the page scrolls or scales the `<pre>` rather than
pretending to reflow a terminal. The history window is 300 polls, i.e. five
minutes at the default 1s interval. The standalone terminal links to the view,
which is the whole of its discovery.

## Risks / Trade-offs

- **The producer is a child process** → the bridge spawns it, restarts it with
  backoff, and kills it on shutdown; if the bridge dies, `run.sh` ends the unit,
  which is the existing lifecycle contract.
- **The image must ship the producer** → the Dockerfile builds `cmd/emb-top`
  (`CGO_ENABLED=0`) beside the bridge and copies it in; no ONNX, no CGo.
- **Frames expose the last-event ticker** (model, text count, latency) → no
  request text is ever carried, and the visitor-facing `MONITOR` refusal is
  unchanged; the ticker is accepted because it is the operational surface the
  view exists to show.
- **SSE bandwidth** → frames are small and the same bytes go to every viewer; the
  platform's compression applies to the stream, and the subscriber cap bounds it.
- **A stuck producer shows a stale frame** → the view states unavailability and
  stops presenting the last frame as current; the producer reports connection
  loss in its own frames.
- **Frame size is fixed while the viewport is not** → the page scales or scrolls
  a monospace block; it does not reflow.

## Migration Plan

Deploy is a rebuild of the sandbox image (`just sandbox-image` / `sandbox-deploy`)
plus the site update for the terminal's link. The new routes are additive; a
rollback is the previous image and page. No data or state migrates; the producer
keeps nothing on disk.
