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
(`#0B0B0B`), one vivid orange (`#FF5A1F`), a muted grey (`#6B6963`) and a
leader grey for the annotation steps. No gradients, no rounded corners, no
shadows except the 5px printed offset under the hero button.

**Type** — three roles, all self-hosted and preloaded. **Inter 900** for the
giant `emb` and the masthead wordmark; **Archivo** (variable, 100–900) for
the claim, the button labels, the feature names and the pipeline labels;
**JetBrains Mono** for every technical label, annotation and paragraph.
Each `@font-face` declares a wide weight range so any requested weight maps
to the single real cut we ship — with `font-synthesis-weight: none` there is
never a fake bold. The body deliberately does **not** set
`-webkit-font-smoothing: antialiased` — the poster's ink is heavier than
macOS antialiasing, and the same face at the same size reads noticeably
lighter with it.

Inter 900 was chosen for the wordmark by measurement, not taste: against
the poster's `emb` it matches stem/x-height `0.350` (poster `0.354`),
`e`/x-height `0.993` (`0.981`), `b`/x-height `1.024` (`1.050`) and
`b`-counter `0.293` (`0.290`), where Archivo Black gives `0.369` /
`1.104` / `1.058` / `0.312` and much wider counters.

**Composition** — the page is *one* two-column spread, not a stack of
full-width bands: the left column carries the claim, the sub, the actions
and the feature list; the right column carries the pipeline and then the
terrain, which the feature list overlaps vertically. That is what keeps the
whole page inside a 1 : 1.33 frame at 1086px wide.

The composition's backbone is the orange signal axis: `.sig{x=182}` inside
the plate SVG, the plate centres from `.pipeline{margin-left}` , the ridge's
branch point and the axis the annotations hang off all sit on it.

**The pipeline** is a real dimetric projection: plate half-width 182,
top-face ratio `.326`, plate pitch 130, thickness 28, shear
`matrix(1 .326 -1 .326 0 0)` on the plan-space `<pattern>` fills. The
signal line is the **last** child of the SVG so it rides on top of the
plates, as on the poster. The INFERENCE plate carries a light→black grid
ramp (`#ramp` + `linearGradient`-driven `<mask>`), and the EMBEDDINGS plate
scatters 74 small isometric blocks tinted through custom properties
inherited into the `<use>` shadow tree. There are no port dots: on the
poster the signal line is the only orange inside the stack.
`tools/gen-isometric.py` emits the whole fragment from those numbers — edit
the constants there, re-run, and paste.

**The terrain** is the photograph knocked out of the paper: an inline SVG
whose `<clipPath>` is the skyline traced from `mountain.jpg` itself (flood
fill from the top edge for bright, low-variance pixels, keeping only the
component that reaches the bottom, then Douglas–Peucker at 5px). The left
and right edges are cut on a diagonal so the frame reads as terrain falling
out of the picture rather than a photograph's rectangle, and the base is
cut by the footer rule. A print screen (the `#print-dots` pattern, clipped
to the same skyline so it never screens the sky) sits over it in `multiply`.

The photograph is put through the screen-print curve
`grayscale(1) brightness(.68) contrast(3)`, over the crop window
1900 × 790 at (140, 590) so the massif sits under the signal spine. The
order matters: `contrast()`
clamps everything above ~0.67 to a single value, so putting it first blew
39% of the massif out to one flat white; `brightness()` first and letting
contrast ramp after it keeps a real gradient through the highlights. Tuned
against boxes that are mountain in both the poster and this photograph
(page x500-900 y1200-1380, and x620-1000 y1150-1340): region means within
~7/255 of the poster's.

> A CSS `mask-image` was tried first and is **not** used here: Chrome fails
> to composite a mask applied before the (large, async) photograph finishes
> loading, which silently renders the terrain invisible. A `clipPath` has no
> such ordering hazard.

**The wordmark** is solid ink carrying an irregular photocopy speckle — an
opaque `speckle.svg` tile with ~2% punched holes used as an alpha mask — not
a regular halftone lattice. Three consequences shape the rule: the mask only
paints inside the element's border box, so `.wordmark` is deliberately
`132vw` wide (otherwise the bleed is silently clipped); the word is set to
`53.9vw` so the x-height lands on the poster's 323px at a 1086px viewport;
and it is tracked `-.067em` and compressed `scaleX(.93)` about its centre.

The tracking is the part that matters most. The poster's logotype is set to
near-zero sidebearings — `e` and `m` touch, `m` and `b` clear each other by
about 13px. Measured at 1086px, Inter's natural gaps are 70px and 13px, so
the lockup takes one extra pull of `-.06em` between `e` and `m` only
(`.wm-e`), which lands the `em` ink on the poster's 735px right edge while
leaving the `m`→`b` gap alone. `scaleX` then buys back the width the
tracking removes so the word still bleeds off both edges.

Known deviation: Inter's ascender/x-height is 1.30 against the poster's
1.425, so the `b` tower is ~46px short at this size, and Inter's `m` arches
are wider (1.47 x-heights against the poster's 1.27) while its `b` counter
is narrower. That is a different cut, not a different setting — Archivo 900
measures 1.343 and Roboto Flex 1.593, both further off, so Inter remains the
closest self-hostable face. Closing it properly needs the original font or
traced letterforms. Its annotation rides the counter of the `b` down to
980px, then becomes a normal annotation under the word.

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
- `mountain.jpg` is a photograph from Unsplash. It is a placeholder for
  licensing review before any public deploy; the skyline clip-path is traced
  from this exact crop (see `tools/gen-isometric.py`), so swapping the
  photograph means re-running the tool. (The reference poster's massif is a
  different photograph — see the addendum in `CORRECTIONS.md`.)
