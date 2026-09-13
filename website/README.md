# emb — product site

The static marketing site for [`emb`](../README.md), designed as a
neo-brutalist technical poster that happens to function as a product
homepage. The brief lives in [`../DESIGN.md`](../DESIGN.md); the measured
corrections that took the page to the reference poster live in
[`CORRECTIONS.md`](CORRECTIONS.md) (implemented).

```
website/
├── index.html              single page: masthead → hero (prose + pipeline)
│                           → protocol → scripts → operations → terrain
│                           → footer
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

**Composition** — the page is a signal travelling from the wordmark to the
massif, and it is organised around one line rather than stacked as bands.

The hero is *one* two-column spread: the left column carries the claim, the
sub, the actions and the feature list; the right column carries the pipeline.
At 1086px wide that hero spread measures 1464px tall — a 1 : 1.348 frame
against the poster's 1 : 1.333 — and the poster's ratio governs the hero
alone. The document is no longer one sheet and its total height is not a
constraint.

Below the hero sit **three blocks**, each a movement rather than a section:

| Block | Ground | Carries |
|---|---|---|
| Protocol | paper | the ruled ledger, a shell specimen, the console plate |
| Scripts | full-bleed `#111110` | the `model(fn(input))` shift, the Lua specimen, the five shipped scripts |
| Operations | paper | the ops ledger and the `emb-top` capture |

Then the **terrain, last**, where the signal lands and the page stops. The
three grounds are the variety; the blocks themselves introduce nothing the
hero did not already use.

The composition's backbone is the orange signal axis:

```text
--fold: 65.31%          /* = --col-prose + 40.7% x (1 - --col-prose) */
--spine-w: calc(var(--plate) * 4 / 364)
```

`--fold` is the hero's own geometry, not a tuned number. `.pipeline` sits in
column 2 of `.hero__body` — which has no `gap`, so column 2 is
`100% - --col-prose` wide — and its spine is at 40.7% of that column
(`40.7% - --plate/2 + --plate/2`, the `--plate` terms cancelling). That makes
the axis width-independent, so every spine segment in the page and the route's
fork can be derived from the one value instead of defended at seven widths.

`--spine-w` exists because the SVG spine is `stroke-width: 4` in a 364-unit
viewBox, so its rendered width scales with whatever the SVG is stretched to.
Above 1000px that is `--plate`; below it the stack column is 50% of the content
box and then the whole of it, and each breakpoint redeclares `--spine-w` to
match. It is not a cosmetic number: measured with the plate-derived value at
600px, the CSS line was **2.406px against a 6.066px SVG stroke** — a 2.5x
weight step at the handover. With the per-breakpoint values the two agree to
**0.009px** (`CORRECTIONS.md`).

The hero keeps its own masked SVG spine (`.sig`), which is what hides the line
behind each plate's front edge; the CSS spine begins where that SVG's box ends.
Every block and the terrain carry a `.spine` segment. Sections have **no
vertical margins, only padding**, so consecutive segments abut exactly — and
where a segment would cross body copy it is either placed in a lane the copy
clears, or hidden behind an opaque plate, which is the rule the hero's plates
already follow.
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

The cut-out and the ridge route share one box (`.terrain__art`), whose aspect
ratio is the artwork's, so neither can stretch relative to the other. The box
is anchored by its **left** edge:

```text
left: calc(var(--fold) - (1124.3 / 2172) * var(--art-w))
```

`1124.3` is the fork's own x in the artwork, so the route's fork lands on the
spine **by construction** rather than by an offset that has to be re-defended
at every width — it replaced a right-anchored `--art-right` that left the spine
and the fork 208px apart at 834px. The right edge is free to bleed past the
shell; `html{overflow-x:clip}` holds it, and `documentElement.scrollWidth`
still equals `clientWidth` at every width measured.

`--band` is `max(--art-h, clamp(...))` where `--art-h` is
`--art-w x 770.2/2172`. The floor matters: it is what guarantees the art box's
top edge — where the route's trunk begins — is never above the band's top,
which is where the spine ends.

On phones and tablets the terrain goes full-bleed (`--art-w` becomes 145vw and
then 124vw, with `--fold` moving to 25% and then 50% to stay on the plates'
centre) and the two slogans sit in the sky above the massif. The route rides
the rock there too, because it lives in the artwork's own pixel space rather
than in a viewport-relative one.

The route is the trunk only. It used to fork three **data branches** across
the sky over the massif — `BLOB OR VALUES`, `HELLO 3` and `1 MS WINDOW` — each
carrying a README fact the pipeline diagram cannot show, and each staged 14%
apart along the scroll so the facts arrived in sequence. They are gone: three
spurs crossing the peak turned the massif into a diagram of itself, and the
three facts already have a home in the blocks, where they sit beside the
command they describe. What is left is one line arriving somewhere, which is
the whole point of the metaphor.

Both slogans carry the paper with them (`background: var(--bg)`). The massif's
silhouette passes under a label at some width in the 320–1440 range, and a
knockout is both the fix and the honest one — no contrast check can certify
text over a photograph. It is the same device the spine gets from the content
above it.

**The blocks** — the poster argues; these prove. Each is built only from the
poster's atoms: a `.block__head` bar, ruled `.cap` entries (a hairline and a
mono ladder, never cards), and dark plates.

**The block header is a solid bar, and it inverts with its ground.** The
heading is not a label with a hairline under it — it is the poster's loudest
typographic move, Archivo at 750 in uppercase at up to 34px, reversed out of
`--fg` on the paper blocks and out of `--bg` on the dark one. That gives the
sequence a beat at every block boundary, makes the dark block read as the page
turned over rather than as a paper page with a dark patch in it, and lets one
element carry a section without a rule. It is the existing display voice used
louder, not a new one.

The bar is opaque, so it covers the spine where it crosses — the same rule the
console plate and the hero's slabs follow. `.block__grid` is two columns whose
split *is* the spine's lane — body copy ends before the line and the mono
facts begin after it, so no text can ever sit on it.

**The dark block is the page's one inversion, made of material already on the
page** — the SERVE plate's own `#111110` and `#292823`, lit by the spine
crossing it in `--accent` at **6.06:1**. There is no new palette here, only
this one turned over. Its `--muted` is remapped to `--rule`, because
`#6B6963` measures only **3.40:1** on `#111110`.

**The console is the SERVE plate, laid flat.** It reuses that plate's own
faces plus the same 1px `--bg` stroke every other plate carries, and it sits
*above* the spine: the line passes behind it and re-emerges below, which is
the rule the hero's plates follow. There are no title bars and no traffic
lights — that would be a costume note from another world.

It introduces **no new colour**. The type lifts existing tokens onto the dark
field, and every value is measured (`:focus-visible` included): `--bg` ink at
**16.59:1**, `--rule` for the dim voice, the badge and the control boundaries
at **9.22:1**, `--accent` for the prompt, the error prefix and the focus ring
at **6.06:1**. `--accent-ink` is the one token that does *not* travel: it is
tuned for the paper (4.66:1) and measures only **3.52:1** here, so the console
overrides the global focus ring back to `--accent`.

**It is a placeholder, and it says so.** The bar reads `DEMO · NOT A LIVE
SERVER`, the note under the panel says the live client is not wired, and there
is no endpoint anywhere. The controls are real — a `<form>`, a labelled input
and a `<pre aria-live>` — so the live version is not a rewrite: replacing
`window.embConsole.exec` with a RESP client drives the same markup, modes and
states. Two modes are the two special functions: `REDIS` shows the bytes and
the `VALUES` envelope, `SCRIPTS` shows a script loaded once and called by SHA
to answer as a classifier. Every line is copied from `README.md` or
`examples/scripts/`, and the SHA1 in the scripts transcript is a real
`sha1()` of `examples/scripts/sst2.lua` — the same value the server's
`scriptSHA()` returns. The executor never touches the network, playback is
line-by-line rather than per character, and `prefers-reduced-motion` collapses
it to one frame. Without JavaScript the form is hidden and a `<noscript>`
transcript stands in.

The panel is a real `role="tablist"` with roving `tabindex` and arrow-key
navigation, and the live region is armed *after* the idle line is painted, so
loading the page does not announce a console hint. Every control clears the
44px target floor at 320–834px. The smallest text in the region is **12px at
1086** and **14px at 390**, which is the committed floor.

**Code is typeset as code.** Every specimen — the shell invocation, the Lua
source, the `model(fn(input))` shift, and the console's replayed commands and
replies — carries four token classes. The specs in the markup are marked by
hand; the console's plain-string transcripts get the identical classes from a
small pattern-based highlighter in `main.js` (four rules, no library), which
is why `sst2` is not mangled into `sst` + `2`: the numeric rule is
word-bounded. Emphasis is weight and colour-role, never a second hue — the
page has one accent and this does not spend it twice. Measured on both
grounds:

| Class | Paper | Ratio | Dark | Ratio |
|---|---|---|---|---|
| plain | `--fg` | 17.28:1 | `--bg` | 16.59:1 |
| `.t-cmd` | `--fg` w700 | 17.28:1 | `--bg` w700 | 16.59:1 |
| `.t-str` | `--accent-ink` | 4.66:1 | `--accent` | 6.06:1 |
| `.t-num` | `--fg` tabular | 17.28:1 | `--bg` tabular | 16.59:1 |
| `.t-dim` | `--muted` | 4.82:1 | `--rule` | 9.22:1 |

A code block's only container is a rule above and below it. It never uses a
coloured side border, which the craft floor refuses and the poster's own
language does not use.

**`emb-top` is a full-width band.** Its capture's longest line is ~100
characters; no 640px column holds that without either cutting the output or
wrapping the heatmap bars mid-run. The panel is the dashboard's real render
from `README.md` with rows omitted, and it is labelled `Sample run`. On phones
the capture wraps (`pre-wrap`) rather than scrolling sideways, so the region
never introduces horizontal scroll.

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
from `stroke-dashoffset` as the terrain rises through the viewport. The
terrain is now the last section before the footer, so the route's progress
denominator (`rect.height * 0.86`) is reached by the page's own end rather
than mid-scroll: at maximum scroll `vh - rect.top` is the band's height plus
the footer's, which clamps the trunk to fully drawn. Reveals are driven by
element position rather than `IntersectionObserver`
intersection, so an anchor jump or a fast flick can never leave content
invisible; a `ResizeObserver` on the root covers the rest of what can move the
trigger line under an element — zoom, rotation, a late font or image — and a
`@media print` block collapses the whole cascade to its end state, because a
print pass never scrolls.

**The one case that is not covered is a capture that neither scrolls nor
resizes.** A `captureBeyondViewport` full-page screenshot lays the document
out without moving the trigger line, so every reveal below the fold is still
at `opacity: 0` and the capture silently omits it — measured: 10 of 20
`[data-reveal]` elements hidden on a fresh load at 390px. A print pass is
safe (`@media print` collapses the cascade); a screenshot is not. `just
website-shot` therefore scrolls the page to the end and back before it
captures, and any hand-rolled capture must do the same. This is the accurate
version of the claim the earlier passes made too broadly.

Groups ripple instead of flipping as a block: each feature, stage and block
entry carries its list position in `--i`, and the slab stagger is keyed by
plate name, so a shared property can never make a hover wait out another
element's delay. Each block ripples on its own, so one block's entrance never
waits out another's.
`prefers-reduced-motion` is honoured throughout.

**Responsive** — the two-column spread holds down to 1000px; below that the
prose, the pipeline and the terrain stack. From 641px to 1000px the four stage
notes stay beside the plates they describe (each note's top is a percentage of
the stack's own height, so it is centred on its plate at every width). Below
640px they become a ruled list directly under the stack, bound to the diagram
by the same `01–04` numbers the plates carry: four annotated layers cannot
share a 350px measure at the 14px floor, so the list is the honest shape for
that width. Phones keep the isometric stack — it is the page's whole
argument.

**The spine is responsive in three states, and `--fold`, `--fold-hero` and
`--art-w` move together in each one** — change one without the others and the
line and the route come apart:

| Range | `--fold` | What the spine does |
|---|---|---|
| ≥1001px | 65.31% | an explicit lane: `.block__grid`'s split *is* the lane, so copy clears it; the console and emb-top plates cover it |
| 641–1000px | 25% | the same lane at the stacked pipeline's plate centre; the massif goes full-bleed at 145vw |
| ≤640px | 90% | the rail moves to the right margin, because a 390px box cannot give both a centre line and a readable measure; everything is held to its left, the plates' own spine is hidden, and the rail threads the plates |

Four measured corrections are baked in. The hero's CSS spine and the diagram
now share **one grid cell** below 1001px: with the spine in a row of its own it
began *below* the stage notes and left a 390px hole after the last plate
(phones) or whenever the notes outgrew the plates (tablets). The stack's paper
covers the line behind the plates, so the masked SVG spine still draws the gaps
rather than the CSS one filling them in.

And **on phones the rail moves to the right margin** (`--fold: 90%`), with the
stage notes, the block grids, the header bars and the emb-top panel all held to
its left:

```css
width: calc(var(--fold) - var(--spine-w) / 2 - var(--fold-gap));
```

At the plate centre a 390px content box gives 175px per side, which left the
notes about **16 characters** — readable in theory and not in the hand. On a
right rail the same content measures **299px** (≈38 characters) and the rail is
still one line. Measured at 390px: notes and block grids 299px, all of them
ending **16px** short of the rail, **zero** text boxes on a visible rail, and no
horizontal scroll; at 320px the rail is at 272 and the notes are 236px.

Two things follow, and both are deliberate:

- **The diagrams's own spine is hidden on phones** (`.pipeline__svg .sig`). There
  is only room for one line: two orange lines 40% apart, both visible in the
  gaps between plates, is not a composition. The rail passes *behind* the plates
  instead — the stack's paper knockout is dropped at this width so the line
  threads them exactly as it does on desktop, visible in the gaps and hidden
  where a diamond covers it.
- **The massif is cropped.** The route's fork is fixed at 51.76% of the artwork,
  so putting it on a right rail necessarily pushes the massif's right slope
  off-frame (`--art-w` grows to `156vw` to keep the left edge at the content
  edge). The summit now sits under the rail, which is where the line arrives —
  the handover is exact (rail 334.98, fork 334.99 at 390px).

Gutters survive a notch: `--pad-l` / `--pad-r` fold in
`env(safe-area-inset-*)`, and everything that cancels a gutter to reach the
sheet's edge cancels those instead.

**The footer is dark** — the same `#111110` as the console, the emb-top
capture and the scripts block, so the page closes on the plate's own material
rather than putting a paper strip back after the photograph. `--muted` is
remapped to `--rule` there for the same reason it is in the dark block
(`#6B6963` is only 3.40:1 on `#111110`); measured, the copy is **9.22:1** and
the `TEXT IN. FLOATS OUT. / EMB` mark is **16.59:1**.

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
