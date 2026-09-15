## Context

See `proposal.md` for motivation and `specs/` for the behaviour contract. What
shapes the approach is the state of the two halves involved:

- `emb-top` is a Bubble Tea TUI (`cmd/emb-top/main.go`). It redraws on its poll
  tick (default 1s), so its content is effectively one frame per second; it needs
  roughly 100+ columns before rows start being dropped, and it truncates model
  names to 13 characters. It has a headless `-once -samples N` mode.
- The plate lives in the operations block of `website/index.html` as
  `.topviz__screen` (`<pre>` of `.tf` spans) on a `#111110` plate, with
  `.topviz__src` reading "Illustrative render · not a measured run".
- The site has no build step and a strict publishing contract:
  `.assetsignore` + `website/tools/published-tree.py`'s explicit `SERVED`
  frozenset + `_headers` (`/assets/img/*` is `immutable`, 1 year). The repo
  already derives hand-typed values instead of typing them (`stamp-version.py`,
  `stamp-presets.py`).
- The site collapses its whole animation cascade under
  `prefers-reduced-motion`; an unconditional animation would be the only motion a
  reader cannot opt out of.
- The dev shell already has `ffmpeg` available transitively; `asciinema`,
  `agg`, `libwebp` and `imagemagick` are not yet declared. Building `emb`/`emb-top`
  and driving load both need the server half (Go, ONNX, `redis-cli`), while the
  renderers belong to the website half.

## Goals / Non-Goals

**Goals:**

- One command, run in the standard dev shell, produces the plate from a real run.
- The artifact is legible enough to read metrics off at the plate's width.
- The plate animates only when the reader allows it, and loses no text to
  assistive technology relative to the `<pre>` it replaces.
- A re-record is a one-command operation that leaves the serving tree exact.

**Non-Goals:**

- No cluster view: the plate is one node. Tiling several `emb-top` panes was
  considered and rejected as a different story.
- No player on the page: the `.cast` is provenance, never served.
- No `emb-top` changes, no docs-site change (`docs/operations.md` keeps its
  text render), no recording in CI.

## Decisions

**Capture: `asciinema record --headless` + `agg`.**
`asciinema 3.2.1` records a pty with no TTY, no browser and a fixed
`--window-size`, and `agg 1.9.0` re-renders the cast with an arbitrary hex theme,
speed, frame-rate cap and time-range selection. The nixpkgs attribute is
`asciinema-agg`; plain `agg` is the unrelated Anti-Grain Geometry 2D library
(checked: the pair is substitutable on aarch64-darwin, so neither builds from
source in the website shell). Alternatives rejected:
`vhs` (runtime Chromium; the flake deliberately avoids browsers it cannot
substitute cheaply), a hand-rendered animation from `emb-top -once` samples (it
would not be a capture of the dashboard), and `asciinema-player` on the page (a
second runtime dependency on the poster, which ships only `main.js`).

**Shipped format: animated WebP in `<picture>`, plus a poster frame.**
``<source srcset="…webp" media="(prefers-reduced-motion: no-preference)">`` with
a `<img>` of the poster as the fallback. GIF was rejected on size; `<video>` was
rejected because a video needs scripted or attribute-level autoplay to be gated
and its text is softer than a lossless-ish WebP for a frame that changes once a
second. `(prefers-reduced-motion: no-preference)` in a `<source media>` is
evaluated at parse time and needs no JavaScript, so the no-script scenario holds.

**Geometry and edit.** `--window-size 120x40` (fits `emb-top`'s rows, leaves room
for the heatmap, both stream charts, four model rows, the gauges and the ticker);
`agg --text-font-family <mono> --font-size 16 --line-height 1.4 --fps-cap 10
--speed 1.5 --idle-time-limit 1 --select <range> --last-frame-duration 2`. The
low frame cap is honest — the content changes once a second — and keeps the file
small. Named time positions in the cast delimit where the interesting window
starts.

**Palette from site tokens.** `agg --theme` takes background, foreground and 16
palette entries as hex triplets, so the recording is rendered in the plate's own
colours rather than a bundled theme: plate `#111110`, paper `#F3F0E8`, and the
accent `#FF5A1F` mapped onto the ANSI slot `emb-top` uses for the request-rate
stream. Because errors are dropped (below), no red slot needs a semantic
guarantee.

**No error narrative.** `emb-top`'s per-model `err/s` comes from `EMB.INFO
<model>` → pool counters, which only move on a real tokenizer or inference
failure; scripted-model failures increment a counter `EMB.INFO` does not report,
and the aggregate error rate is computed in the sampler but never rendered in the
TUI. The illustrative plate's `err 18 ↑` therefore cannot be reproduced on
purpose, and the recording omits errors rather than staging one.

**Models: local paths, short names, one wide model.** A committed config under
`website/tools/topviz/` names three or four models by local `onnx`/`tokenizer`
path (no HuggingFace fetch during a take), with aliases at most 13 characters so
the dashboard does not truncate them, and at least one model with a different
dimension so the per-model identity line varies.

**Load: a committed `redis-cli` script, backgrounded inside the recording.**
The wrapper `asciinema` records runs `emb-top` in the foreground while the
scenario script runs in the background with its output redirected, so the ramp is
on camera and nothing but the dashboard is visible. A shell script over
`redis-cli` was chosen over a new Go command (avoids adding a binary and a
deadcode surface) and over the Ruby bench harness (it targets different models
and asserts budgets rather than telling a story). The scenario staggers per-model
start and stop, mixes text lengths, and repeats texts for cache hits.

**Naming and the page reference: content-hashed, written by one helper.** The
animated WebP and poster are named `emb-top-<sha8>.{webp,png}` from their own
bytes, so the existing immutable rule is honest and `_headers` needs no change. A
small `website/tools/topviz/publish.py` writes the names into `index.html` and
into `published-tree.py`'s `SERVED` set, deletes the previous take, and prints
the sizes. Two files must agree on the names, and a half-write is exactly the
class of defect `published-tree.py` exists to catch, so one helper owns both
writes rather than `sed` in the recipe.

**Shell split.** `asciinema`, `agg` and `libwebp` join `websiteDeps` so
`nix develop .#website` can render; the `just` target refuses to run without the
server half (`go`, `redis-cli`, an ONNX runtime path) and says so, since it must
build and drive `emb`.

## Risks / Trade-offs

- **The recorder toolchain may not be substitutable** → verified on
aarch64-darwin: the closure carries the two Rust packages as `-vendor` /
`-vendor-staging` derivations and `nix develop .#website` copies `agg-1.9.0`
from `cache.nixos.org` rather than building it. Note that the AGENTS.md
grep for `\.source.*\.drv$` does **not** catch a Rust package that would build
from source — it sees `-vendor` derivations — so this was confirmed by watching
the develop actually copy the path.
- **The recording cannot match the page's mono font**: the site self-hosts
  JetBrains Mono as `woff2`, which `agg` cannot load → accept the system mono for
  now; committing a TTF under the unserved tools tree later is a one-line change.
- **The take is not byte-reproducible**: rates depend on the machine → the name
  hashes the bytes and the caption states the date; the narrative is structural
  (idle → load → drain), never numeric.
- **The dashboard is steppy**: it redraws once per second → frame cap and idle
  limit keep the file small; smoothness is not the goal, readability is.
- **Editing can clip the narrative**: `--select` may cut the drain or the ramp →
  the task list includes watching the shipped asset once, and the cast is kept so
  the range can be re-rendered without re-recording.
- **Roughly half a gigabyte of models** must be on disk to record → the target
  fails before recording and names `just download-model <repo> <dir>`.
- **`published-tree.py` passes only if the rig is the sole writer of the take's
  name** → the publish helper deletes the previous take first; `just
  website-published` is the check.
- **`stamp-version.py`'s docstring cites the element this change removes** → its
  behaviour is unaffected (the ledger and docs keep their markers), but the
  example is updated so the tool does not document a page that no longer exists.

## Migration Plan

The plate is served from the same tree it is edited in, so there is one step: run
`just website-topviz`, then `just website-published` and `just website-ink
website-version-check`, then deploy. Rollback is restoring the removed `<pre>`
from git; the rig is additive except for that one block and the dead `.tf*` rules.

## Open Questions

- Whether to commit a TTF so the recording's type matches the page's mono.
- The exact shipped clip length (the take is ~30 s; the cut lands somewhere
  between 15 and 22 s depending on how the ramps read).
