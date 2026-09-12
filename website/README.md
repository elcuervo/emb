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
│       ├── terrain-v2.png  generated rock formation used by the page
│       ├── terrain-v2.md   generation prompts and provenance
│       ├── mountain.jpg    original unused terrain photograph
│       ├── speckle.svg     original photocopy speckle asset
│       └── og.png          1200×630 social card
├── tools/
│   └── gen-isometric.py    regenerates the pipeline SVG
└── README.md
```

## Run it

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
branch point and the axis the annotations hang off all sit on it.

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

**The terrain** uses `terrain-v2.png`, a generated dry-rock formation guided
by the supplied poster. It spans 78vw on desktop and bleeds beneath the
feature column. `grayscale(1) brightness(1.08)` makes the image's light
background white, and `mix-blend-mode: multiply` blends it into the cream
paper. The landscape must not establish an isolated stacking context, which
would make the image background opaque. The responsive route stays aligned
with the tablet stack; mobile annotations sit above the rocks.

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
flick or a full-page capture can never leave content invisible.
`prefers-reduced-motion` is honoured throughout.

**Accessibility** — real `<header>`, `<nav>`, `<main>`, `<section>`, `<h1>`,
`<h2>`, `<h3>`, `<footer>`; the giant `emb` is `aria-hidden` decoration and
the semantic `<h1>` is the value proposition. The pipeline is announced by a
visually hidden `<h2>` that the four stage `<h3>`s hang off, and the terrain
is decorative.

## Assets

- Fonts are self-hosted WOFF2 subsets (latin) from Google Fonts.
- `terrain-v2.png` was made with the built-in image generation tool using the
  supplied poster as a visual reference. Prompts and provenance are recorded
  in [`assets/img/terrain-v2.md`](assets/img/terrain-v2.md).
- `mountain.jpg` is the original Unsplash placeholder, retained but unused
  by the page. Its original licensing review caveat still applies to reuse.
