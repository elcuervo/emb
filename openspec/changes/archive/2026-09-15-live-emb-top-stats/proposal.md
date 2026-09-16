## Why

The sandbox at `cli.emb.is` is a real `emb` node answering real visitor traffic,
but nothing on the site shows that. `emb-top` already renders that traffic as a
live dashboard; the sandbox can expose the dashboard's own frames, read-only, so
a visitor sees the operational surface working instead of only reading about it —
and because the frames are the dashboard's own output, there is no second
dashboard to build or keep honest.

## What Changes

- **`emb-top` gains a headless frame mode.** Alongside the TUI and `-once`, it
  renders the dashboard's complete frames (color included) to stdout on the poll
  interval, with no TTY, no alternate screen, and no keyboard input. The frames
  are the same `View()` output the TUI paints, so the live view cannot drift from
  the terminal.
- **The sandbox bridge serves a read-only live view at `/stats`.** One `emb-top`
  producer runs per sandbox and every viewer receives the same frames, so the
  stream costs one poll per second regardless of how many people watch. The view
  keeps a 5-minute window (300 polls at the default 1s interval).
- **The frame carries the dashboard's own color.** The producer forces a color
  profile rather than inheriting the pipe's, so the frames are not silently
  stripped to monochrome.
- **The view is read-only by construction.** The page carries no controls and the
  stream endpoint accepts no input; there is no path from a browser back to
  `emb`, and the existing spend bounds and allowlist are untouched.
- **The standalone terminal links to it.** `cli.emb.is/` gains a link to the live
  view so the easter egg is reachable without knowing the URL.
- **No new dependency and no pty.** The producer is the same pure-Go dashboard
  code; the bridge does not import the TUI, so the server-side process stays a
  web process and the `termenv` background query never runs behind the bridge.

## Capabilities

### New Capabilities

- `sandbox-live-stats`: a read-only, live view of an `emb-top` frame stream on
  the sandbox origin, with one producer serving every viewer.

### Modified Capabilities

- `emb-top-dashboard`: adds a headless frame-streaming mode that emits the
  dashboard's complete rendered frames without a terminal.

## Impact

- `cmd/emb-top/` — the headless frame mode and its flag.
- `website/repl/` — the `/stats` page and `/api/stats` stream, the producer's
  supervision, the terminal-code-to-markup rendering of a frame, and the
  embedded page; the bridge's `Dockerfile` builds and ships the producer.
- `website/repl/index.html` — the link to `/stats`.
- `justfile` / `AGENTS.md` — the bridge's check surface for the new page and
  stream.
- No change to the server, the RESP surface, the allowlist, the spend bounds, the
  presets, the gems, or the models.
