## Why

The landing page has to do two jobs at once and does the second one badly. It
has to make a developer want the product (the poster: wordmark, exploded
pipeline, one orange axis, massif) and it has to tell them how to use it (install,
config, every command, the Lua surface, ops, benchmarks). Twelve correction
passes have polished the first job to fractions of a pixel while the second is
still missing its foundation: **the page never says how to install the thing**,
carries **no measured number** behind "at massive speed", and ships three claims
the repository does not support — an image input path that does not exist
(`examples/scripts/siglip2.lua:1-5` feeds `pixel_values` as zeros), "Production
ready." on a `0.4.0.pre4` product, and an `emb-top v0.4.0` capture against
`VERSION` `0.4.0.pre4`.

Two things make this the moment. The landing's own brief already ranks
"implementation details" sixth, after the CTA (`DESIGN.md` visual hierarchy), so
the detail was never supposed to live there. And the first browser measurement of
the composed page found a rendering defect the previous eleven passes could not
see: the four stage notes are held 10px past the right sheet edge, so their text
ink is **sliced at 1070–1150px and 1300–1340px — including at 1086px, the
design's own reference frame** (worst 8.89px; the comma in "Raw text, documents,"
and the period in "optimized execution." are cut). Every pass measured
`scrollWidth === clientWidth`, which `overflow-x: clip` guarantees regardless.

## What Changes

- **A new `/docs` surface** (`website/docs/index.html`) carrying the specifics:
  install → first vector, commands, replies and protocol, configuration, the Lua
  API, operations, benchmarks, clients, and the pre-1.0 status line. It is
  derived from `README.md`'s existing structure rather than invented, and
  `README.md` stays the source of truth.
- **The landing keeps its post; it loses its manual.** Detail that both surfaces
  could carry moves to `/docs` only: the YAML config block, the full command
  tables, the reply-format prose beyond the one line that carries the argument,
  the `emb-top` figures, and every version string.
- **The landing gains what a reader needs before clicking**, and nothing more:
  one copy-paste install line, the pre-1.0 status and platform list, exactly one
  measured `BENCHMARK.md` figure with its reproduction command, and exactly one
  link to `/docs`.
- **The landing's unsupported claims are removed, not relocated.** `images` is
  deleted from the hero sub and the INPUT stage note (`/docs` documents the Lua
  image story honestly, including the siglip2 caveat); "Production ready."
  becomes the pre-1.0 status; the `emb-top` capture's version string is generated
  from `VERSION` rather than typed.
- **Three rendering defects are fixed**, each with the regression test that
  would have caught it:
  1. Note ink is contained — `.note{right:-10px}` no longer lets text reach the
     clipped 10px band.
  2. A `main.js` load failure can no longer blank the page — `html.js` currently
     hides the four plates, the spine, and the notes permanently if the script
     never runs, because only `main.js` adds `is-live`/`is-ready`/`.in`.
  3. The print stylesheet stops hiding dark-surface text — `.block--dark`,
     `.console`, `.topviz__screen` and `.footer` print `--bg` text on a white
     ground with no `print-color-adjust`.
- **`styles.css` is split into shared and page layers** so `/docs` can inherit
  the world instead of forking it. A second page that links today's sheet would
  inherit the hero's derived `--fold` axis and its spine geometry, which `docs`
  has no composition for.
- **Amended prior decisions.** The `product-site` requirement that the page carry
  one merged capability region below the terrain was already superseded by
  `recompose-site-spine`; this change amends the composition requirement again by
  moving detail off the landing entirely.

## Capabilities

### New Capabilities

- `product-docs`: the `/docs` surface — its information architecture, its
  inheritance of the poster's tokens and floors, its source-of-truth contract
  against `README.md`, and its pre-1.0 honesty rules.

### Modified Capabilities

- `product-site`: the landing's content-truth rules tighten (no input path that
  the shipped scripts do not implement; every version string generated from
  `VERSION`; every measured number traceable to `BENCHMARK.md`; a pre-1.0 status
  statement; one install line and one `/docs` link as the only always-present
  detail), plus three new rendering requirements (text ink stays inside the
  sheet at every width; the composed page survives a `main.js` load failure; the
  printed page keeps its dark surfaces legible).

## Impact

- `website/docs/index.html` — new surface.
- `website/index.html` — hero sub and INPUT note copy; pre-1.0 status and a
  single install line in the hero action area; masthead `Docs` target moves from
  the GitHub anchor to `/docs`; the `emb-top` version string; three spine defects.
- `website/assets/css/styles.css` — shared/page layer split (`styles.css` keeps
  tokens, base, and primitives; the hero-only axis and spine stay with the hero),
  the note-ink containment rule, and the print fix.
- `website/assets/js/main.js` — the `html.js` failure guard; `/docs` reuses the
  file unchanged (its blocks are already feature-guarded).
- `website/README.md`, `website/CORRECTIONS.md` — the split's structure, the
  browser-measured pass, and the two stale claims the audit found (the plates do
  not carry `01–04`; the leaders are hidden below 641px).
- `README.md` — gains a pointer to `/docs` as the hosted form of the same facts;
  no content is removed, since the repository README remains the source of truth.
- `openspec/specs/product-site/spec.md` and a new
  `openspec/specs/product-docs/spec.md` — via this change's deltas.
- Sequencing: `recompose-site-spine` is 33/33 but unarchived, so its
  `product-site` delta has not been synced. This change MUST NOT be archived
  before it (`design.md` D8).
- No server, protocol, gem, or Go/Ruby changes. No new dependency, font, or CDN:
  `/docs` is static HTML in the same world, and the no-build/`file://` promise
  holds.
