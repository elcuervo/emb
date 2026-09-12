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
│   ├── fonts/              self-hosted Archivo (+ Black) and JetBrains Mono
│   └── img/
│       ├── mountain.jpg    terrain photograph (Unsplash)
│       ├── speckle.svg     photocopy speckle mask for the giant wordmark
│       └── og.png          1200×630 social card
├── tools/
│   └── gen-isometric.py    regenerates the pipeline + terrain SVG fragments
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
(`#0B0B0B`), one vivid orange (`#FF5A1F`), a muted grey (`#77756F`) and a
leader grey for the annotation steps. No gradients, no rounded corners, no
shadows except the 5px printed offset under the hero button.

**Type** — three roles. Archivo Black for the giant `emb`, stretched
`scaleX(1.18)` and bled off both edges so the word is wider than the paper;
Archivo 700 for the claim, the button labels, the feature names and the
pipeline labels; JetBrains Mono for every technical label, annotation and
paragraph. The body deliberately does **not** set
`-webkit-font-smoothing: antialiased` — the poster's ink is heavier than
macOS antialiasing, and the same face at the same size reads noticeably
lighter with it.

**Composition** — the page is *one* two-column spread, not a stack of
full-width bands: the left column carries the claim, the sub, the actions
and the feature list; the right column carries the pipeline and then the
terrain, which the feature list overlaps vertically. That is what keeps the
whole page inside a 1 : 1.33 frame at 1086px wide.

`--spine-x: 65%` is the backbone. It is the same line for the hero spine,
the centre of all four plates, the ridge's branch point and the axis the
annotations are hung off.

**The pipeline** is a real dimetric projection: plate half-width 197,
top-face ratio `.3046`, plate pitch 130, thickness 22, shear
`matrix(1 .3046 -1 .3046 0 0)` on the plan-space `<pattern>` fills. The
signal line is the **last** child of the SVG so it rides on top of the
plates, as on the poster. The INFERENCE plate carries a light→black grid
ramp (`#ramp` + `linearGradient`-driven `<mask>`), and the EMBEDDINGS plate
scatters 42 small isometric blocks tinted through custom properties
inherited into the `<use>` shadow tree. `tools/gen-isometric.py` emits the
whole fragment from those numbers — edit the constants there, re-run, and
paste.

**The terrain** is the photograph knocked out of the paper: an inline SVG
whose `<clipPath>` is the skyline traced from `mountain.jpg` itself (flood
fill from the top edge for bright, low-variance pixels, keeping only the
component that reaches the bottom, then Douglas–Peucker at 5px). The left
and right edges are cut on a diagonal so the frame reads as terrain falling
out of the picture rather than a photograph's rectangle, and the base is
cut by the footer rule. A 3px print screen sits over it in `multiply`.

> A CSS `mask-image` was tried first and is **not** used here: Chrome fails
> to composite a mask applied before the (large, async) photograph finishes
> loading, which silently renders the terrain invisible. A `clipPath` has no
> such ordering hazard.

**The wordmark** is solid ink carrying an irregular photocopy speckle — an
opaque `speckle.svg` tile with ~2% punched holes used as an alpha mask — not
a regular halftone lattice. Its annotation rides the counter of the `b`.

**Motion** — things move because data is moving. The spine draws downward,
the four slabs (INPUT → INFERENCE → EMBEDDINGS → SERVE) activate in
sequence, and the ridge route is drawn from `stroke-dashoffset` as the
terrain rises through the viewport. Reveals are driven by element position
rather than `IntersectionObserver` intersection, so an anchor jump, a fast
flick or a full-page capture can never leave content invisible.
`prefers-reduced-motion` is honoured throughout.

**Accessibility** — real `<header>`, `<nav>`, `<main>`, `<section>`, `<h1>`,
`<h2>`, `<h3>`, `<footer>`; the giant `emb` is `aria-hidden` decoration and
the semantic `<h1>` is the value proposition. The four pipeline stages are
an ordered list, and the terrain is decorative.

## Assets

- Fonts are self-hosted WOFF2 subsets (latin) from Google Fonts.
- `mountain.jpg` is a photograph from Unsplash. It is a placeholder for
  licensing review before any public deploy; the skyline clip-path is traced
  from this exact crop (see `tools/gen-isometric.py`), so swapping the
  photograph means re-running the tool.
