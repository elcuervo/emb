# The `emb-top` plate's recording rig

`just website-topviz` records a real `emb-top` session against a real node and
publishes three things from it: the landing page's plate, the still frame behind
it, and the documentation's capture. This directory is the rig: it is under
`website/` because the plate is part of the site, and it is never served
(`/tools/` is ignored, and `published-tree.py` asserts it).

```
just website-topviz                 # build, record, publish all three, verify
just website-player                 # re-vendor the player (pinned in flake.nix)
```

## What one run does

```
bin/emb (4 models, cache on)  ──▶  asciinema record (headless, 120x32)
        ▲                                   │  raw: 5s of blank + the dashboard + the exit
        │                                   ▼
   traffic.sh (redis-benchmark)      trim.py  ── take: the dashboard's own screen, t=0 at first paint
        behind emb-top in a pty             │
        │                                   ├────────────────┬───────────────────┐
        └── 32s, staggered bands            ▼                ▼                   ▼
                                     the take          trim.py --until-pct 65  agg (the site's palette)
                                     (served, replayed      │                   │
                                      by the player)        ▼                   ▼
                                            │         the page's still      docs/assets/
                                            │         frame (fallback)      emb-top-<sha8>.gif
                                            └────────► publish.py ◄────────────┘
                                     (names, placement, caption, figures, reference)
```

Three artifacts, one take:

| artifact | where | why that form |
|---|---|---|
| the trimmed take, replayed by the vendored player | the landing page's plate | the dashboard's own text, in colour, moving — the panel exists to show a live dashboard |
| the same run's frame, held in the plate | the same plate, behind the player | a reader with scripting off or a reduced-motion preference gets the dashboard, not a blank plate |
| the animated capture | `docs/assets/`, referenced by `docs/operations.md` | the documentation surface renders markdown and no scripts, so there the motion has to be a picture |

Files here:

| file | what it is |
|---|---|
| `models.yaml` | the node that gets recorded: four models, all preloaded, cache on, `:16379` |
| `traffic.sh` | the load: one `redis-benchmark` band per model, staggered start and stop |
| `run.sh` | what `asciinema` runs: the dashboard in front, the traffic behind it |
| `trim.py` | cuts the raw recording to the dashboard's alt-screen session (and, with `--until-pct`, to a moment inside it) |
| `publish.py` | writes the take, the frame, the caption and the figures into the page, the served set in `published-tree.py`, and the capture into the documentation |
| `runs/` | the last run's `raw.cast`, `take.cast`, `frame.cast`, `frame.txt`, `node.log`, `traffic.log`, `samples.txt` — gitignored, unserved, and the only way to check what the artifacts show |

## The plate is the recording, and the frame is what it falls back to

The page's plate plays `take.cast` — the recording trimmed to the dashboard's own
session — with the asciinema player. It is not a drawing of the dashboard, and not
a picture of one: the player draws the recording's own characters as text (the
canvas inside it carries the background cells — the heatmap, the gauge bars — and
every glyph is DOM text), so the plate is selectable and stays sharp at any zoom.

The frame in the same plate is not dead weight from an earlier revision. It is
what the page shows when the player cannot run: scripting off, `prefers-reduced-
motion: reduce`, a player file that will not load, or a take that will not parse
(`main.js` puts the frame back on the player's `error` event). It paints before
any script runs, so the plate is never empty and never wrong, and the player
replaces it only where it can actually play something.

The frame sits beside the player's mount, not inside it: the player builds its
terminal into the element it is given and empties that element first, so anything
mounted there is gone before the take even loads. The mount is a bare `div`
carrying `data-topviz-cast`; `main.js` hides the frame when the player starts and
puts it back — hiding the emptied mount — if the take fails. That last path is
the one to re-check after touching the page: point a copy of `index.html` at a
cast that does not exist and confirm the frame returns.

The player itself is vendored: `website/assets/{js,css}/asciinema-player-<version>.*`,
copied verbatim from the release tarball pinned in `flake.nix` by
`just website-player`, with `just website-player-check` to re-derive and compare
them. The site has no build step and no runtime third party: these bytes ship.

## The frame is the dashboard's own text

`asciinema convert -f txt` replays a cast into a terminal and dumps the screen —
so the still frame is not a drawing of the dashboard, and not a picture of one,
but its output. `trim.py --until-pct 65` is what makes that frame worth showing:
the last frame of a take is the drain, where every rate reads zero, and the first
is an empty node. 65% lands in the sustained phase, with all four models active
and the dashboard's window largely filled.

## Five things that are not obvious

**The first five seconds of every take are blank, on purpose.** Bubble Tea's
package init calls `lipgloss.HasDarkBackground()`, which asks the terminal for its
background colour and waits out `termenv.OSCTimeout` — five full seconds — when
nothing answers. A recording pty never answers; a real terminal replies in
microseconds. So `run.sh` waits 8 seconds before loading the node, and `trim.py`
throws the wait away by rebasing the take to the first painted frame. Reading
asciicast **v3** times as if they were v2's absolute ones gives a take that
starts in the past: v3 carries *deltas*, so `trim.py` accumulates, rebases and
re-deltas.

**The traffic schedule may not read the wall clock.** One take was emptied
halfway through by the machine's clock stepping forward 17 seconds: every band
computed its deadline from `date +%s`, saw it in the past and quit at the same
instant. `traffic.sh` is therefore driven by `sleep` alone — the one timer a shell
has that does not follow the wall clock — with the parent owning the timeline and
each band a plain loop it starts and kills.

**The take is ~850 KiB and compresses to ~16 KiB, so its type is declared.** A
recording is escape sequences and per-cell colour, and the dashboard's heatmap
re-emits them every poll: the file is big and extremely repetitive. `_headers`
therefore sets `Content-Type: text/plain; charset=utf-8` on `/assets/cast/*`,
which is true of asciicast (UTF-8 JSON, one event per line) and is on
Cloudflare's list of compressible types. Pages infers the type from the
extension, and an extension it does not know ships with no `Content-Type` at all
— so the take would go over the wire uncompressed, fifty times the bytes it needs.

**The capture has no errors because none can be staged.** `emb-top`'s per-model
`err/s` reads `EMB.INFO <model>` → pool counters, which only move on a genuine
tokenizer or inference failure; a scripted model's failures increment
`script_errors` in `EMB.STATS`, which `EMB.INFO` does not report. There is no
request that raises a model's error rate on purpose, so both artifacts show the
error column at zero rather than inventing a spike — this is why the hand-drawn
render this replaced carried `err 18 ↑` under a "not a measured run" caption.

**The gauges and the heatmap are emb-top's own colours, not a theme artefact.**
The three gauges are drawn as full-width cells and the heatmap as background
cells, with the heatmap's eight steps hard-coded as truecolor in
`cmd/emb-top/main.go` (`heatColors`); the frame around them asks for 256-colour
index 63, which no 16-colour theme can remap — which is why both artifacts show
the same blue frame, and why the player's theme defines all sixteen entries
rather than the few the text uses. `mem` reads `1.0kMB` rather than `1.0GB`
(`fmtRate` stops at `k`). All of it is the dashboard's, and out of this change's
scope.

## Recording it again

`just website-topviz` is idempotent: it rewrites the page's frame, its caption
and its figures, writes a take and a capture named from their own bytes, deletes
the previous pair, rewrites the reference in `docs/operations.md` and the take's
entry in `published-tree.py`'s served set, and ends by running the site's
published-tree check. Both names change on every take, which is what keeps
GitHub's and Cloudflare's caches honest across a re-record.

After a re-record, glance at both surfaces. Their shape is structural (idle →
ramp → all four models → drain) and does not depend on the machine, but their
*rates* do: a busy laptop halves them. Everything about the run that a reader can
check is in `runs/`.

The capture is rendered at font-size 12 — 881px wide, about the width
`docs/operations.md` is read at — which is ~100 KiB smaller than a 14px render
GitHub would only downscale to the same apparent size. If it needs to be smaller
still, the levers are that size and the length of the take, not dropping it.

The models are ~500 MB on disk. The rig fails before recording, naming the file
and the command, when one is missing:

```bash
just download-model Xenova/bge-small-en-v1.5 ./models/bge-small
just download-model Xenova/jina-embeddings-v2-small-en ./models/jina-small
just download-model Xenova/e5-small-v2 ./models/e5-small
```

`website/README.md`'s tools list is the inventory this directory appears in.
