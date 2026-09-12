# emb — product site

The static marketing site for [`emb`](../README.md), designed as a
neo-brutalist technical poster that happens to function as a product
homepage. The brief lives in [`../DESIGN.md`](../DESIGN.md); the measured
corrections that took the page to the reference poster live in
[`CORRECTIONS.md`](CORRECTIONS.md) (implemented).

```
website/
├── index.html              single page: masthead → hero (prose + pipeline
│                           + terrain) → footer
├── assets/
│   ├── css/styles.css      tokens → primitives → sections → responsive
│   ├── js/main.js          entrance sequencing, reveals, pipeline, route
│   ├── fonts/              self-hosted Archivo, Inter and JetBrains Mono
│   └── img/
│       ├── terrain-matte.png  the cut-out the page ships (2172×724, alpha)
│       ├── terrain-v2.png  generated rock formation, the matte's source
│       ├── terrain-v2.md   generation prompts and provenance
│       ├── mountain.jpg    original unused terrain photograph
│       ├── speckle.svg     original photocopy speckle asset
│       └── og.png          1200×630 social card
├── tools/
│   ├── gen-isometric.py    regenerates the pipeline SVG
│   ├── gen-terrain-matte.py derives the terrain cut-out from terrain-v2.png
│   └── png_lib.py          dependency-free PNG reader/writer for the above
└── README.md
```

## Render it

No build step, no dependencies, no CDN. Any static file server works:

```bash
just website            # http://localhost:8080
just website port=9000  # or pick a port
```

Or directly:

```bash
python3 -m http.server 8080 --directory website
```

Opening `index.html` over `file://` works too — nothing here needs a server.

`nix develop .#website` (or the combined `nix develop`) provides the tooling:

```bash
just website-browser                  # once: fetch Chrome for Testing
just website-shot                     # full-page screenshot of :8080
just website-shot http://localhost:8080 /tmp/phone.png 390x844
```

The site's dependency list is `websiteDeps` in [`../flake.nix`](../flake.nix) —
separate from the server's, and browser tooling lives there rather than in the
Go shell. `firefox` is deliberately not on it: nixpkgs builds it from source on
`aarch64-darwin`, which is hours. The check that catches that is in
[`../AGENTS.md`](../AGENTS.md).

## Design notes

**Palette** — a warm off-white ground (`#F3F0E8`), near-black ink
(`#0B0B0B`), one vivid orange (`#FF5A1F`), a muted grey (`#6B6963`) and a
leader grey for the annotation steps. No gradients, no rounded corners, no
shadows except the 5px printed offset under the hero button.

**Type** — self-hosted Archivo for the claim, JetBrains Mono for technical
copy, buttons and labels, and Inter 900 for the masthead brand. The giant
wordmark uses native SVG outlines matched to the reference, so its silhouette
does not depend on font metrics. Fonts are preloaded; no CDN is needed.

**Composition** — the page is *one* two-column spread, not a stack of
full-width bands: the left column carries the claim, the sub, the actions
and the feature list; the right column carries the pipeline and then the
terrain, which the feature list overlaps vertically. That is what keeps the
whole page inside a 1 : 1.33 frame at 1086px wide.

The composition's backbone is the orange signal axis: `.sig{x=182}` inside
the plate SVG, the plate centres from `.pipeline{margin-left}` , the ridge's
branch point and the axis the annotations hang off all sit on it. The terrain
route re-enters the artwork at that same axis (`x1124.3` of 2172), so the
spine, the fork and the ridge read as one line.

**The pipeline** uses a dimetric projection: half-width 182, top-face ratio
`.326`, plate pitch 119 and thickness 24. It carries 46 input marks and 48
small extruded blocks. The INFERENCE plate transitions to a dark lattice;
SERVE has black top and side faces. SVG grain adds a light print texture.
Lifted wire grids, faint surface grids, cube shadows and ruled slab edges
provide depth. The orange signal uses a mask so it enters each plate and
passes behind the front edge. To regenerate the SVG directly in the page:

```bash
nix develop --command python3 website/tools/gen-isometric.py --write
```

Without `--write`, the generator prints the SVG to standard output.

**The terrain** ships as `terrain-matte.png`: a real alpha cut-out derived
from the generated `terrain-v2.png` by `tools/gen-terrain-matte.py`, which
bakes the old `grayscale(1) brightness(1.08)` grade into the alpha channel
(`alpha = 1 - L/white` over black). Compositing it over the paper is
pixel-identical to the `mix-blend-mode: multiply` it replaced (mean error
0.33/255), without the failure mode that came with it: a blend with no
backdrop in its stacking context paints the artwork's own off-white ground as
an opaque rectangle until something forces a repaint. Regenerate with:

```bash
python3 website/tools/gen-terrain-matte.py --write
```

The cut-out and the ridge route share one box (`.landscape__art`), whose
aspect ratio is the artwork's, so neither can stretch relative to the other.
That box is 78vw wide with a 1.8vw bleed, which puts the route's fork on the
signal axis at every width from 1001px to the shell's 1720px cap and beyond
(worst measured error 0.09px), and makes `--band` exactly 27.66vw of artwork:
no distortion, no drift. Retune the three together or not at all.

On phones and tablets the terrain goes full-bleed and the two annotations sit
in the sky above the massif; the route rides the rock there too, because it
lives in the artwork's own pixel space rather than in a viewport-relative one.

**The wordmark** is an inline SVG with three optical outlines in a
1086 × 480 viewBox. At the reference width, the `b` tower starts at y87,
the x-height at y218, and the bowls finish at y567. The `e` terminal, `m`
arches and `b` counter follow the supplied poster. A restrained SVG noise
filter and a diagonal crease provide the ink texture. The counter caption
is decorative and is omitted below 980px when it becomes too small.

**Motion** — things move because data is moving. The spine draws downward,
the four slabs (INPUT → INFERENCE → EMBEDDINGS → SERVE) activate in
sequence, and the ridge route is drawn from `stroke-dashoffset` as the
terrain rises through the viewport. Reveals are driven by element position
rather than `IntersectionObserver` intersection, so an anchor jump, a fast
flick or a full-page capture can never leave content invisible. Groups ripple
instead of flipping as a block: each feature and stage carries its list
position in `--i`, and the slab stagger is keyed by plate name, so a shared
property can never make a hover wait out another element's delay.
`prefers-reduced-motion` is honoured throughout.

**Responsive** — the two-column spread holds down to 1000px; below that the
prose, the pipeline and the terrain stack. Phones keep the isometric stack
(it is the page's whole argument) and read the four stages as a ruled list.
Gutters survive a notch: `--pad-l` / `--pad-r` fold in
`env(safe-area-inset-*)`, and everything that cancels a gutter to reach the
sheet's edge cancels those instead.

**Accessibility** — real `<header>`, `<nav>`, `<main>`, `<section>`, `<h1>`,
`<h2>`, `<h3>`, `<footer>`; the giant `emb` is `aria-hidden` decoration and
the semantic `<h1>` is the value proposition. The pipeline is announced by a
visually hidden `<h2>` that the four stage `<h3>`s hang off, and the terrain
is decorative.

## Assets

- Fonts are self-hosted WOFF2 subsets (latin) from Google Fonts.
- `terrain-v2.png` was made with the built-in image generation tool using the
  supplied poster as a visual reference. Prompts and provenance are recorded
  in [`assets/img/terrain-v2.md`](assets/img/terrain-v2.md). It is kept as the
  source for `terrain-matte.png`, which is what the page loads.
- `mountain.jpg` is the original Unsplash placeholder, retained but unused
  by the page. Its original licensing review caveat still applies to reuse.
