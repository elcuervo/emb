## Why

The `emb-top` plate on the landing page was a hand-built `<pre>` of spans whose
own caption read "Illustrative render · not a measured run". It is now a capture
of a real run — but a *still* one, and the panel exists to prove a dashboard
moving under load. The one thing a reader cannot get from a frozen frame is the
thing that makes `emb-top` worth showing.

Meanwhile the repo has everything needed to show the real thing the way it
actually renders: `asciinema` records headlessly with no browser, the take is
already trimmed to the dashboard's own screen (`runs/take.cast`), and
`asciinema-player` replays a cast as a terminal of real text in the page. There
is no reason for the site's copy to be a picture of a terminal when it can be
the terminal.

The goal is a plate a reader can *read metrics off* and *see move*. That is a
text requirement before it is a picture one: a capture of 12–13px type shown at
the plate's measure is softer than the type itself and costs ~400 KB to be
softer, while the recording's own output is text at every scale. Motion and text
were treated as a trade in the previous revision; they are not — a player draws
the cast as text, so the landing gets both. The GIF stays in the documentation,
because that surface renders Markdown on GitHub and cannot replay a cast.

## What Changes

- `just website-topviz`: one command that builds the binaries, starts a single
  `emb` node with several models and the cache on, warms it, records `emb-top`
  headlessly under a scripted ~30 s traffic scenario, and publishes from that one
  take:
  - the landing page's plate, which **plays the take** — the trimmed cast,
    served under a content-hashed name and replayed by a self-hosted, pinned
    `asciinema-player` — with the dashboard's own **text frame** at its busiest
    representative moment kept in the same plate as the still state;
  - the **animated GIF** of the same run, committed under `docs/assets/` and
    referenced from `docs/operations.md` (unchanged from the current revision:
    the documentation keeps its animation).
- **The plate animates**: the site no longer argues that it must be still. It
  animates only when the reader allows motion: the player is created
  by `main.js` when `prefers-reduced-motion` is `no-preference`, with play/pause
  controls; otherwise the plate holds the captured frame — as it also does with
  scripting unavailable, since the frame is markup and the player is not.
- **BREAKING (site markup and served tree)**: `.topviz__screen` stops being a
  bare `<pre>` and becomes the plate the player renders into; the site serves
  three new assets — the player's JS and CSS (vendor, versioned filenames) and
  the take's cast (content-hashed) — so `published-tree.py`'s served set grows by
  three lines and `_headers` gains a content-hashed-cast rule. The `.sr-only`
  figures sentence and the caption stay exactly where they are.
- The GIF path is untouched: `docs/operations.md` trades its hand-drawn ASCII
  render for the real capture, with a provenance line stating the date, the
  captured version, the model count and the node.
- Keep the rig's findings where they belong: the README explains why the capture
  contains no errors, why every take opens on five blank seconds, and why the
  traffic schedule may not read the wall clock — and now also why the cast the
  page serves needs no pre-processing to match the GIF's pacing.

## Capabilities

### New Capabilities

- `emb-top-capture`: a reproducible, scripted capture of the `emb-top` dashboard
  under load — one command, a deterministic traffic scenario, the dashboard's own
  recording replayed as text on the page with a still frame behind it, and an
  edited, themed, animated GIF for the docs, whose names change when their bytes
  do.

### Modified Capabilities

- `product-site`: the "`emb-top` panel shows real output" requirement changes
  again. The panel is no longer a still: it is the run's own recording, played as
  text, with the captured frame as its still state. It is still not a picture —
  the player draws the cast with the page's own type — but it may move, so the
  motion requirement flips from "the panel MUST NOT animate" to "the panel
  animates only when the reader allows it, and can be stopped".

## Impact

- `website/index.html` — the plate keeps its element, its measure and its
  caption, gains the player's mount point and its still frame, and loads the
  player's stylesheet and script (landing only).
- `website/assets/js/main.js` — creates the player from the plate's stamped cast
  path, under the same `prefers-reduced-motion` gate the rest of the page uses.
- `website/assets/css/styles.css` — the plate's frame rules stay; the player's
  own chrome is sized and themed to the plate (site palette, one-cell leading).
- `website/assets/js/asciinema-player-<version>.min.js`,
  `website/assets/css/asciinema-player-<version>.css` (new, vendored, pinned),
  `website/assets/cast/emb-top-<sha8>.cast` (new, content-hashed, written by the
  rig).
- `website/tools/published-tree.py` — plus three served entries (two vendor, one
  take) and the existing no-`.js`/`.css`-under-immutable assertion, which is why
  the vendor files keep their revalidating rules.
- `website/_headers` — a `/assets/cast/*` immutable rule for the content-hashed
  cast; nothing else moves.
- `docs/operations.md`, `docs/assets/` — the animated GIF, committed and
  referenced, exactly as the current revision has them.
- `website/tools/topviz/`, `justfile` (`website-topviz`, `website-player`),
  `flake.nix` (`websiteDeps`, the pinned player tarball), `website/README.md`
  (the tools inventory and the vendor provenance).
- No server, protocol, client or `emb-top` behaviour changes.
- Design work runs through the Impeccable workflow, as `product-site` requires.
