## Context

See `proposal.md` for motivation and `specs/` for the behaviour contract. What
shapes the approach is the state of the two surfaces involved:

- `emb-top` is a Bubble Tea TUI (`cmd/emb-top/main.go`). It redraws on its poll
  tick (default 1s), so its content is one frame per second; it needs roughly 100
  columns before rows are dropped, and it truncates model names to 13 characters.
  It has a headless `-once` mode, and its screen is ordinary terminal output — so
  a recording of it is, in the end, text.
- The plate lives in the operations block of `website/index.html`, on a `#111110`
  plate, at `clamp(12px, 1.02vw, 13px)` in the page's own mono face. The take is
  a 120×32 grid, so the plate is the whole dashboard at that measure: no crop, no
  downscale of a raster, and nothing to re-render when the reader zooms.
- The rig already produces the take the page needs: `runs/take.cast` is the
  recording trimmed to the dashboard's alt-screen session, rebased so the first
  painted frame is t=0 (`trim.py`). The GIF is rendered from that same cast.
- `docs/operations.md` is the repository's operations reference (rendered by
  GitHub), and it carries a hand-drawn ASCII render of the dashboard — invented
  models, invented rates, and an invented error spike.
- The site has no build step and a strict publishing contract: `.assetsignore`,
  `published-tree.py`'s explicit `SERVED` set, `_headers`. It already vendors
  third-party bytes (three self-hosted `woff2` faces) and already ships one
  script (`assets/js/main.js`, 18 KB) that collapses its own animation cascade
  under `prefers-reduced-motion`.
- The dev shell declares `asciinema`, `asciinema-agg` and `libwebp` in
  `websiteDeps`; the player itself is not packaged in nixpkgs, so it has to be
  vendored from a pinned release.

## Goals / Non-Goals

**Goals:**

- One command, run in the standard dev shell, produces every artifact from one run.
- The page's plate is legible at every scale, is the run's own output, and moves.
- The reader keeps the motion choice: autoplay only where motion is welcome, a
  way to stop it, and a still frame where scripting is not available.
- The documentation's capture shows the same run in colour, at full size.
- A re-record is a one-command operation that leaves every surface exact.

**Non-Goals:**

- No cluster view: one node. Tiling several `emb-top` panes was considered and
  rejected as a different story.
- No baked frame sequence: the page plays the cast rather than a pre-rendered
  strip of frames (see Decisions).
- No player on the docs surface: GitHub renders markdown, not scripts, which is
  exactly why the animated GIF stays there.
- No `emb-top` changes, no recording in CI.

## Decisions

**Capture: `asciinema record --headless` + `agg`.** `asciinema 3.2.1` records a
pty with no TTY, no browser and a fixed `--window-size`; `agg 1.9.0` renders the
cast with an arbitrary hex theme, speed and frame-rate cap. The nixpkgs attribute
is `asciinema-agg` — plain `agg` is the unrelated Anti-Grain Geometry library.
Alternatives rejected: `vhs` (runtime Chromium, which this flake avoids) and a
hand-rendered animation from `-once` samples (not a capture).

**The plate plays the cast; the documentation keeps the GIF.** This is the
change's central decision, and it is about what each surface can render:

| | what it can render | what it gets |
|---|---|---|
| the landing plate | scripts, CSS, fonts — and the reader's motion preference | the run's recording, played as text at the plate's measure, with a still frame behind it |
| `docs/operations.md` | Markdown on GitHub — no scripts | `agg`'s themed GIF of the same cast |

The previous revision sent the animation to the docs because the plate "must be
read". That reasoning was about *raster* captures: at this measure a 1175px frame
of 12–13px type is 14px type and softer, for ~400 KB. A replay is not a raster.
`asciinema-player` draws the cast with real text in the DOM at the plate's own
size, so the reader gets crisp, selectable characters *and* motion, and the page
pays for the take itself rather than for a picture of one — ~850 KiB of
asciicast, which is escape sequences and per-cell colour, delivered as the ~16 KiB
it compresses to (`_headers` declares the type that makes that happen). The docs
surface cannot replay a cast at all, so it keeps the GIF — one run, two forms,
each the best one its surface can render.

**The player is vendored and pinned, not fetched.** The site has no build step
and no CDN: a `<script src="https://…">` would put a third party in the critical
path of the plate, and would make what the plate shows depend on someone else's
uptime and version — the one off-origin request this site makes is
`cli.emb.is/terminal.js`, the project's own sandbox module, which is the console's
whole subject rather than a dependency borrowed for a component. So the two
release files (`asciinema-player.min.js`, 181 KB, and `asciinema-player.css`,
19 KB — both from the monolithic bundle, which embeds its terminal emulator and
needs no second request or WASM file) are committed under
`website/assets/{js,css}/asciinema-player-<version>.{min.js,css}`, with the
release tarball pinned by hash in `flake.nix` and a `just website-player` target
that re-vendors from that store path and asserts the bytes. Committed bytes keep
the no-build-step property; the pin keeps a version bump a hash change rather
than a download-from-wherever. The version in the filename is what makes the
revalidating `/assets/js/*` rule honest: published-tree.py forbids `.js`/`.css`
under an immutable rule, and a bump changes the URL anyway.

**The cast is served under a content-hashed name, and its type is declared.**
`website/assets/cast/` gains `emb-top-<sha8>.cast`, written by `publish.py` from
`runs/take.cast`; the
previous one is deleted, the player's stamped path is rewritten, and the served
entry in `published-tree.py` is rewritten with it. Same rule as the GIF, same
reason: a stable name would be cached by a CDN after a re-record. `_headers`
gains `/assets/cast/*` as immutable, which is only true because the name is the
content — and gains an explicit `Content-Type: text/plain; charset=utf-8` with
it. That is not decoration: Pages infers the type from the extension, an
extension it does not know ships with no type at all, and a response without one
is not on Cloudflare's list of compressible types. These bytes are ~850 KiB of
terminal output that gzip to ~16 KiB, so the type is what makes the plate cost
sixteen kilobytes instead of eight hundred and fifty.

**The still frame stays, as the plate's base state.** The cast alone cannot be
read with scripts off, and a reader who asked for reduced motion should get a
dashboard, not a blank plate. So the captured frame — already part of the rig for
the previous revision — remains in the page inside the plate, and `main.js`
hides it and shows the player only when motion is welcome and the player script
is present. That ordering is deliberate: the frame is what paints first, so
there is no empty plate, and no-JS is a complete state rather than a degraded
one. It also means the plate's `<pre>` is still the thing `publish.py` stamps.

The frame sits *beside* the player's mount rather than inside it, which is not
cosmetic: the player builds its terminal into the container it is given and
empties it first, so a frame mounted there would be destroyed on the way in and
could not be restored on failure. `data-topviz-cast` therefore rides on a
separate, empty `div`; a take that will not load disposes the player, hides the
mount and unhides the frame, which is verified by pointing a copy of the page at
a cast that does not exist.

**Motion is opt-in, and stoppable.** The player is created with
`autoplay: !reduceMotion`, `loop: true` and `controls: true`. Anything that moves
for more than five seconds without a pause control is a failure of WCAG 2.2.2,
so the player's own control bar is kept — themed to the plate — rather than
suppressed for looks; it is also the only way a reader who *wants* to scrub the
take can. The site's existing `matchMedia('(prefers-reduced-motion: reduce)')`
listener is the same gate the rest of the page uses, so the plate does not
introduce a second motion policy. Two more things the plate does not pay for:
where the plate is hidden (below the narrow breakpoint, the run's figures stand
in for it) the player is never created, so a phone does not fetch a take it
cannot see; and if the take will not load, the player is disposed and the frame
comes back rather than leaving an empty plate.

**Pacing comes from the player, not from a second cast.** `agg` renders the GIF
sped up with a low idle limit; the player takes the same options at runtime
(`speed`, `poster`), so the page and the docs show the same take at the same pace and the rig never has to keep two pre-processed copies of one
recording.

**Palette from site tokens.** The player themes through CSS custom properties,
so the plate's take is drawn in the page's own colours: plate `#111110`, paper
`#F3F0E8`, the accent `#FF5A1F` on the request-rate stream — the same mapping the
`agg --theme` triplets use for the GIF. The one thing the still frame cannot
carry is the heatmap's colour; the player carries it, so the plate no longer
loses that encoding.

**No error narrative.** `emb-top`'s per-model `err/s` reads `EMB.INFO <model>` →
pool counters, which only move on a real tokenizer or inference failure, while a
scripted model's failures increment a counter `EMB.INFO` does not report. The
hand-drawn render's `err 18 ↑` therefore cannot be reproduced on purpose, and
both artifacts show the error column at zero rather than staging a failure.

**Models: local paths, short names, one wide model.** `website/tools/topviz/models.yaml`
names four models by local `onnx`/`tokenizer` path (no HuggingFace fetch during a
take), with aliases at most 13 characters so the dashboard does not truncate
them, one model of a different width, and `intra_op_threads: 3` per session —
four sessions at the default oversubscribed this machine three times over, which
pinned the recorded p95 at the top of its chart.

**Load: a committed `redis-cli`-driven scenario, scheduled by `sleep` alone.** The
parent process owns the timeline and starts and kills each band; no band reads a
clock, because the wall clock stepped 17 seconds during one take and every band
whose deadline was computed from `date +%s` quit at the same instant.

**Shell split.** `asciinema`, `asciinema-agg` and `libwebp` join `websiteDeps`,
alongside the pinned player tarball; the `just` target refuses to run without the
server half (Go, `redis-cli`, an ONNX runtime path), since it must build and
drive `emb`. Serving the site still needs none of it: the player's bytes are
committed and the cast is in the tree.

## Risks / Trade-offs

- **The page gets heavier**: 181 KB of player script plus 19 KB of stylesheet,
  against the still frame's zero → both are cached and revalidated, the take is
  ~850 KiB of text that compresses to ~16 KiB over the wire while the GIF it
  duplicates is 490 KiB, and the scripts load off the critical path (`defer`)
  only on the landing. If this ever outweighs what the motion proves, the lever
  is the plate's own animation, not the take.
- **Third-party code enters the served tree** → pinned by hash in `flake.nix`,
  versioned in the filename, provenance in `website/README.md`, and the take is
  the only thing it is pointed at.
- **The cast is raw terminal output** (an operator's take includes whatever the
  pty saw) → it is the trimmed take, not the raw recording: `trim.py` cuts it to
  the dashboard's alt-screen session, and the rig's `runs/` stay unserved.
- **The player's theme could drift from the page's tokens** → the mapping lives
  in one place in `styles.css` next to the plate's own rules, and the GIF's
  `agg --theme` triplets are the reference for it.
- **A re-record can leave a stale cast referenced** → the cast's name is its
  content, `publish.py` rewrites the reference and deletes the previous file, and
  `published-tree.py` fails if a served take is not the referenced one.
- **The take is not byte-reproducible**: rates depend on the machine → the cast
  and the GIF are named from their bytes and the caption states the date; the
  narrative is structural (idle → ramp → all models → drain), never numeric.
- **The five-second first paint** (`termenv.OSCTimeout`, unavoidable from
  `main`) and **asciicast v3's delta timing** are documented in the rig's README,
  because both cost a re-derivation if forgotten.

## Migration Plan

Run `just website-topviz`, then `just website-published`, `just website-ink` and
`just website-version-check`, then deploy. Rollback is `git revert`: the rig is
additive, the plate is one block of one page, and the vendor files are two files
deleted with it.

## Open Questions

- Whether the hosted docs page (`website/docs/index.html`), which carries prose
  about `emb-top` and no render at all, should carry the same player. It is a
  different surface with its own constraints (it ships no JavaScript at all), and
  out of scope here.
