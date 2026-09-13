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
(`#0B0B0B`), one vivid orange (`#FF5A1F`) for surfaces, a deeper
`--accent-ink` (`#C23D00`) for the accent used as text or as a focus ring, a
muted grey (`#6B6963`) and a leader grey for the annotation steps. No
gradients, no rounded corners, no shadows except the 5px printed offset under
the hero button. `#FF5A1F` is 2.74:1 against the paper and `--accent-ink` is
4.66:1, which is why the accent never carries text or a ring at its surface
value.

**Type** — self-hosted Archivo for the claim, JetBrains Mono for technical
copy, buttons and labels, and Inter 900 for the masthead brand. The giant
wordmark uses native SVG outlines matched to the reference, so its silhouette
does not depend on font metrics. Fonts are preloaded; no CDN is needed.

Every technical label is clamped so it cannot compute below **12px on
`vw`-driven layouts**, and below 1000px the same labels are set at **14px**
explicitly. Both are floors in the stylesheet rather than wishes: the
smallest text on the 1086px frame is 12px and the smallest text on a phone is
14px, measured.

**Composition** — the page is *one* two-column spread, not a stack of
full-width bands: the left column carries the claim, the sub, the actions
and the feature list; the right column carries the pipeline and then the
terrain, which the feature list overlaps vertically. At 1086px wide that
measures 1464px tall — a 1 : 1.348 frame against the poster's 1 : 1.333, so
the sheet runs 16px long at the reference width.

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
passes behind the front edge.

The plates are painted **back to front** — SERVE first, INPUT last — because
the camera sits about 19 degrees above the horizon and the highest plate is
therefore the nearest one. The other order leaves each lower plate's top face
cutting into the front skirt of the plate above it: a 73 x 24 unit wedge
around the spine, hidden today only because the signal line covers its centre.
`data-depth` on each `.slab` records the level (1 farthest, 4 nearest), and
the generator emits them in that order. The activation stagger is keyed by
plate name, so document order and animation order are independent. To
regenerate the SVG directly in the page:

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

Three **data branches** leave the trunk above the fork and run through the sky
over the massif, each with a margin annotation carrying a fact the pipeline
diagram cannot show: `BLOB OR VALUES` (the reply formats), `HELLO 3` (RESP2
to RESP3 on the same connection) and `1 MS WINDOW` (the batcher). The trunk
draws across the whole scroll travel and each branch then runs over the
following 58% of it, 14% apart, so the facts arrive in sequence. Below 1000px
the branches and their annotations are dropped together: a phone has no paper
beside the massif, and an unlabelled spur crossing the peak reads as an
artefact.

**The wordmark** is an inline SVG with three optical outlines in a
1086 × 480 viewBox. At the reference width, the `b` tower starts at y87,
the x-height at y218, and the bowls finish at y567. The `e` terminal, `m`
arches and `b` counter follow the supplied poster. A restrained SVG noise
filter and a diagonal crease provide the ink texture. The counter caption
is decorative and is omitted below 1000px when it becomes too small.

**Motion** — things move because data is moving. The hero entrance is one
authored sequence: the masthead, the annotations, the wordmark and then the
prose and actions land 90ms apart, settling at 780ms while the spine finishes
its 850ms draw at 1000ms. The four slabs (INPUT → INFERENCE → EMBEDDINGS →
SERVE) then activate in name-keyed order when the pipeline crosses 0.85
viewport heights, and the ridge route plus its three data branches are drawn
from `stroke-dashoffset` as the terrain rises through the viewport. Reveals
are driven by element position rather than `IntersectionObserver`
intersection, so an anchor jump or a fast flick can never leave content
invisible; a `ResizeObserver` on the root covers the rest of what can move the
trigger line under an element — zoom, rotation, a late font or image — and a
`@media print` block collapses the whole cascade to its end state, because a
print pass never scrolls. Groups ripple instead of flipping as a block: each
feature and stage carries its list position in `--i`, and the slab stagger is
keyed by plate name, so a shared property can never make a hover wait out
another element's delay.
`prefers-reduced-motion` is honoured throughout.

**Responsive** — the two-column spread holds down to 1000px; below that the
prose, the pipeline and the terrain stack. From 641px to 1000px the four stage
notes stay beside the plates they describe (each note's top is a percentage of
the stack's own height, so it is centred on its plate at every width). Below
640px they become a ruled list directly under the stack, bound to the diagram
by the same `01–04` numbers the plates carry: four annotated layers cannot
share a 350px measure at the 14px floor, so the list is the honest shape for
that width. Phones keep the isometric stack — it is the page's whole
argument. Gutters survive a notch: `--pad-l` / `--pad-r` fold in
`env(safe-area-inset-*)`, and everything that cancels a gutter to reach the
sheet's edge cancels those instead.

**Accessibility** — real `<header>`, `<nav>`, `<main>`, `<section>`, `<h1>`,
`<h2>`, `<h3>`, `<footer>`; the giant `emb` is `aria-hidden` decoration and
the semantic `<h1>` is the value proposition. The pipeline is announced by a
visually hidden `<h2>` that the four stage `<h3>`s hang off (the notes carry
`role="list"` so VoiceOver and Safari keep the list semantics that
`list-style: none` removes), and the terrain is decorative. Focus is a 2px
`--accent-ink` ring at 2px offset — 4.66:1 against the paper, where the
surface accent is only 2.74:1. The note ⇄ plate emphasis is decoration that
no information depends on: the stage number and label are always visible, and
it answers to hover, to tap and to a second tap elsewhere, so touch is not
left out. Every rendered control is at least 44px tall at 320–834px.

## Assets

- Fonts are self-hosted WOFF2 subsets (latin) from Google Fonts.
- `terrain-v2.png` was made with the built-in image generation tool using the
  supplied poster as a visual reference. Prompts and provenance are recorded
  in [`assets/img/terrain-v2.md`](assets/img/terrain-v2.md). It is kept as the
  source for `terrain-matte.png`, which is what the page loads.
- `mountain.jpg` is the original Unsplash placeholder, retained but unused
  by the page. Its original licensing review caveat still applies to reuse.
