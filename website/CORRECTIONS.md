# Corrections — matching the site to the poster reference

> **Status: implemented.** Every item below has been applied to
> `index.html`, `assets/css/styles.css` and `tools/gen-isometric.py`. The
> measured after-state at a 1086px viewport is recorded at the end of this
> file; the numbers below are the *before* measurements that drove the work.
>
> **Second pass (see the addendum at the end).** A later polish pass re-measured
> the poster with an objective method (same-string ink widths, not eyeballed
> grid overlays) and found the masthead, nav, CTA, meta-band tracking and
> feature-list numbers in this file to be wrong. The addendum lists what
> changed; the first-pass numbers are kept for history.

Current reference: `/tmp/rootshell-uploads/drop-20260912-180449.png`.
The fourth-pass addendum below supersedes earlier implementation notes.

Original source: `/tmp/rootshell-uploads/drop-20260911-195139.png`
(referred to below as **the poster**; frame is **1086 × 1448**, aspect **1 : 1.333**).

Method: the poster frame was rendered with a pixel grid overlay, and the current
site was rendered at the same 1086 px width with headless Chromium, then compared
region by region (`ref.png` vs `site.png`). Every number below is a measurement in
**poster pixels** at 1086 px viewport width; divide by 1086 to get a percentage of
viewport width, which is what the layout should use.

Confidence: geometry, type roles, colours and the composition are high confidence.
Sub-pixel stroke weights and the exact mono size are ±1 px.

## The one-line diagnosis

The site is a faithful execution of `DESIGN.md`, but the poster is **one
two-column composition**, not a stack of full-width bands. The poster fits
*masthead → hero → pipeline → features + landscape → footer* into a 1:1.33
frame; the current page is 1:1.81 at the same width. Nearly every other
correction is downstream of that difference.

---

## 0. Reference measurement table (1086 px wide)

```
MASTHEAD        0 – 62, hairline rule at 62 (light grey, 1px)
  brand         x 37–102, display face, ~30px
  vrule         x 121, y 17–46 (1px, light grey)
  tagline       x 146–238, mono ~10px, 2 lines, lh 12
  nav           DOCS 379–408 · GITHUB 437–479 · MODELS 507–548 · COMMUNITY 577–627
                (mono ~11px, gap 29px, block centre 503 = 46.3%)
  CTA           x 742–875, y 18–48 (133 × 30), flat black, sentence-case sans
                "Get Started →" ~15px, white, no offset shadow

HERO META       y 78–205, NO bottom border
  left          x 40, y 133–172, mono ~20px, ls .08em, 2 lines (pitch 29)
  middle        x 443, y 108–163, mono ~11.5px, 4 lines (pitch 15)   ← starts ABOVE the left block
                short rule 25 × 2 at y 181
  right         right-aligned to x 1046, y 105–175, mono ~20px, pitch 29
                short rule 33 × 2 at y 181 (dark, replaces the orange em dash)

SPINE          x 698–703 (5px), i.e. 64.4% — runs y 76 → 1130 (mountain peak)
   … is drawn IN FRONT of the pipeline plates

WORDMARK       ascender top 212, x-height top 250, baseline 540
               'e' right edge 310 · 'm' right edge 745 · 'b' counter centre (905, 400)
               bleed: 'e' clipped at x 0, 'b' clipped at x 1086
               texture: fine irregular photocopy speckle, reads as solid black

CLAIM          x 40, baselines 618 / 659 / 700 / 741 (pitch 41), x-height ~25.7
               "embedding server" = 367px wide (current: 346px — see C2.1)
SUB            x 40, baselines 812 / 833 / 855 (pitch 21.5), mono ~13px, max-w ~375
               wraps as: "Drop-in compatible. Production ready." / "Turn text, images,
               and more into vectors" / "at massive speed."
HERO CTA       x 44–269, y 875–933 (225 × 58), orange, black offset shadow 5px r/d
               label "Get Started →" sans bold ~21px, sentence case
GH LINK        x 317–410 text (mono ~14px, sentence case) + glyph 416–437
               2px grey underline, not black

FEATURES HEAD  text x 40–195 (mono bold ~11px, ls .16em) at y ~995
               rule 220 → 301 (81px fixed, 1.5px) — NOT flex-grow
FEATURES       name baselines 1034 / 1093 / 1152 / 1211 / 1270 / 1329  (pitch 59)
               icon 33px at x 45 · name x 100, sans bold ~14px · desc 12px mono muted
               icons are engraved/duotone (outline + hatch/dot fill), not clean 1.9px vectors

PIPELINE       plates centred on the spine (x 700)
               half-width 197 (x 505–900) · half-height 60 · thickness 27
               top-face ratio 60/197 = 0.30  (current code: 0.375)
               port/plate pitch ≈ 128 (ports at y ≈ 634 / 736 / 872 / 1001)
               stack box ≈ 395 × 500 (current render: 345 × 610)
NOTES          numbers x 902 (mono 12px, grey) · labels x 943 (sans bold ~16px, ls .07em)
               desc x 943 (mono 12px, grey, lh 15)
               label baselines 613 / 728 / 861 / 984 · 3-segment step leaders, 1px grey
PLATE COLOURS  INPUT light + dot field · INFERENCE ramps light→BLACK grid · VECTORS light + cubes
               SERVE solid black + light lattice ✓ (already correct)
CUBES          ~40 blocks, tall extrusion, mixed sizes, spread across the plate

LANDSCAPE      knockout silhouette on paper — NO plate, NO halftone, NO vignette
               base meets the footer rule; bleeds off the right edge
               left base (390, 1448) · peak (700, 1130) · right edge (1086, 1345)
ROUTE          ONE solid 4px orange polyline on the ridge (no dashed ghosts,
               no crosshair marks), branching from the spine at the peak
ANNOTATIONS    left  x 412–500, y 1152–1195 (mono ~11.5px, dark), rule 33×2 at 1215
               right x 962–1055, y 1155–1220, rule 33×2 at 1240
               "+" registration mark x 962–1000, y 1090–1128
               … they sit INSIDE the features/landscape band, beside the feature list
NO CREDIT      no "IMAGE — UNSPLASH", no "PLATE 01 · SIGNAL PATH" caption in the poster

FOOTER         rule at y 1388 (full width, 1px) · band 60px
               left  "© 2024 emb. Open source, forever."  x 40  (muted)
               right "TEXT IN. FLOATS OUT.  /  EMB"      to x 1046 (ink)
```

---

## C0 — Structural: rebuild the lower half as one two-column composition

**Problem.** `.landscape` is a sibling of `.hero` and spans the full width below
everything, which is why the page is 500px taller than the poster. In the poster
the mountain lives in the *right column*, starting directly under the pipeline and
running to the footer, while the feature list continues down the left column.

**Change `index.html`** — move the whole `<section class="landscape">` *inside*
`.hero__body`:

```html
<div class="hero__body">
  <div class="hero__prose"> …claim, sub, actions, features… </div>
  <div class="hero__pipeline">
    …pipeline stack + notes…
    <section class="landscape" aria-labelledby="scale-h"> …annotations + plate… </section>
  </div>
</div>
```

The annotations band is no longer a full-width flex row: in the poster
"EMBED EVERYTHING FURTHER" sits **left of the mountain** (x 412) and
"HIGHER DIMENSIONS BRIGHTER APPLICATIONS" **right of the peak** (x 962), both
inside the landscape block, absolutely positioned over it.

**CSS** (replaces the current `07 landscape` block):

```css
.hero__body{ grid-template-columns: repeat(12, minmax(0,1fr)); align-items: start; }
.hero__prose{ grid-column: 1 / 6; }
.hero__pipeline{ grid-column: 6 / 13; }

.landscape{
  position: relative;
  margin-top: clamp(24px, 3vw, 60px);
  /* reaches past the shell gutter to the right edge of the viewport */
  margin-right: calc(-1 * var(--gut));
}
.landscape__plate{
  position: relative;
  height: clamp(230px, 26vw, 360px);   /* was 280–520px full-bleed */
  overflow: hidden;
  margin-right: 0;
}
```

Delete `.landing__annotations` as a band, `.landing__spine`, `.landscape__credit`,
`.landscape__caption`, `.landscape__halftone` and the three colour-wash gradients
in `.landscape__plate::after`. The spine simply continues into the route.

**Result.** Feature list and mountain overlap vertically (features end at y 1375,
mountain spans 1086→1448), removing ~400px and putting the footer one screen down.

---

## C1 — Geometry

### 1.1 Spine

```css
--spine-x: 64.4%;   /* was 58.7% */
```

This single number moves the hero spine, all four plate ports and the route
branch point. Everything else stays relative to it, so nothing else needs
re-deriving.

### 1.2 Pipeline stack proportions

The poster's plate is flatter and the stack is tighter than the current SVG.

| quantity | current (`viewBox 0 0 320 610`) | poster (→ viewBox units) |
|---|---|---|
| top-face half-height / half-width | 60 / 160 = 0.375 | 0.30 → **48** |
| plate-to-plate pitch | 141 | **104** |
| plate thickness | 15 | **17** |
| stack width : height | 345 : 610 | **395 : 500** |

Regenerate the four `<g class="slab">` paths from those numbers rather than
hand-editing (the shear `matrix(1 0.375 -1 0.375 0 0)` on `#p-dots`, `#p-grid`
and `#p-fine` becomes `matrix(1 0.30 -1 0.30 0 0)`), and set the SVG box to
something like `viewBox="0 0 400 520"` with
`.pipeline__stack` holding a fixed `max-width: clamp(300px, 36vw, 395px)`.

### 1.3 Signal z-order

`.sig` is the first child of the SVG, so the plates paint over it. In the poster
the orange line is clearly visible **on top of** the INPUT, INFERENCE and SERVE
plates. Move the `<line class="sig">` to the end of the SVG.

---

## C2 — Typography

### 2.1 Claim: same face and size, but it is tracked in too far

Measured against the rendered font (via `measureText` on the real woff2, 1086 px
viewport):

| | poster | current |
|---|---|---|
| "embedding server" ink width | **367 px** | 346 px |
| x-height | **25.7 px** | 25 px |
| baseline pitch | **40–41 px** | 41 px |

Same face, same size, same leading — the 6 % width difference is almost exactly
the `letter-spacing: -.03em` in the current rule (16 glyphs × .03em at 42.4 px
≈ 20 px). Archivo 700 at 100 px measures 858.7 px for that string; with the
current `-.03em` that predicts 344 px, matching what the page renders.

So **do not switch to Archivo Black** (it is 14 % wider than Archivo 700 at the
same size and would overshoot). Fix the tracking and drop the dead
`font-stretch`:

```css
.claim{
  font-family: var(--sans);
  font-weight: 700;                 /* 800 if it still reads light after 2.5 */
  font-stretch: normal;             /* no-op today — see note */
  font-size: clamp(2rem, 3.95vw, 4.7rem);
  line-height: .95;                 /* 40–41 px pitch at 1086 */
  letter-spacing: -.005em;
}
```

> Verified: `archivo-var-latin.woff2` has **no `wdth` axis** — `font-stretch`
> 62.5 % / 100 % / 125 % all measure identically — so every `font-stretch` in
> the stylesheet (including `.claim{font-stretch:106%}` and
> `.hero__prose`'s relatives) is dead code. The `wght` axis does work
> (100→759.4, 400→797.7, 700→858.7, 900→961.1 for "embedding server").

### 2.2 Sub-paragraph

```css
.hero__sub{
  max-width: 29ch;                  /* ≈375px at 1086 — forces the poster's 3-line break */
  font-size: clamp(12.5px, 1.2vw, 15.5px);
  line-height: 1.65;
}
```

### 2.3 Global ink weight

`body{ -webkit-font-smoothing: antialiased }` thins every glyph on macOS. The
poster reads noticeably heavier at the *same* font and size — compare the
masthead `emb` or the claim in the side-by-side: same letterforms, more ink.
Drop the declaration (or set `auto`) and, as a second lever,
`text-rendering: optimizeLegibility` → `geometricPrecision` is not needed.

```css
body{ -webkit-font-smoothing: auto; }
```

### 2.4 Mono scale

The poster's mono runs slightly larger in the meta band and slightly smaller in
the labels. Two edits: raise `.hero__meta-a` / `.hero__tech` to
`clamp(15px, 1.85vw, 26px)` with `line-height: 1.45`; drop `.rule-head` and
`.nav a` by ~1px and keep `letter-spacing` at `.13em`.

### 2.5 Meta band alignment

`.hero__meta-b` currently has `padding-top:.5em`, pushing it below the left
block. In the poster it *starts above it* (y 108 vs 133): remove the padding and
align the band with `align-items: start`.

Remove the `.hero__meta` bottom border — the poster has no rule under the meta
band. Also remove `.tech-label__dash` (the orange em dash) and instead give both
`.hero__meta-b` and `.hero__tech` a `.meta-rule` child:

```css
.meta-rule{ display:block; width:33px; height:2px; background:var(--fg); margin-top:12px; }
```

---

## C3 — Components

### 3.1 Buttons — sentence case sans, not uppercase mono

Poster: **"Get Started →"**, Archivo bold ~21px, `text-transform:none`,
`letter-spacing:0`. Current: `GET STARTED →`, JetBrains Mono 700, uppercase.

```css
.btn{
  font-family: var(--sans);
  font-weight: 700;
  font-size: clamp(14px, 1.5vw, 21px);
  letter-spacing: 0;
  text-transform: none;
  gap: 1.1em;
  padding: .95em 1.6em;             /* 225 × 58 hero, 133 × 30 masthead */
}
.btn--dark{ /* masthead */ padding: .5em .9em; font-size: clamp(12px,1.1vw,15px); }
.btn--accent{ --shadow-s: 5px; }    /* keep — poster shadow measures 5px */
```

Both labels are sentence case in the poster; the hero button's arrow is a plain
`→` with ~24px of separation from the text.

### 3.2 "View on GitHub"

Poster is `View on GitHub` (sentence case, mono ~14px) with a **2px grey**
underline. Current is uppercase with a 1px black underline.

```css
.ghlink{
  text-transform: none;
  font-size: clamp(12px, 1.15vw, 14px);
  border-bottom: 2px solid var(--muted);
  padding-bottom: 3px;
}
```

### 3.3 Masthead

- Rule colour: the poster's hairline is light grey → `border-bottom-color: var(--rule-soft)`.
- `@media (max-width:1240px)` currently hides `.masthead__tagline` and `.vrule`;
  the poster shows both at 1086. Move that breakpoint to `max-width: 980px`.
- The nav block is centred at 46.3 % of the viewport, not 50 %. Cheapest fix:
  `.masthead__inner{ grid-template-columns: auto 1fr auto }` with the nav
  `justify-self: center; margin-right: 7%`, or drop the 3-column grid and place
  the nav with an explicit `grid-column: 4 / 10`.

### 3.4 Features

- `.rule-head__rule`: replace `flex: 1 1 auto` with `flex: 0 0 81px`.
- Row pitch 59px, icon 33px, name `clamp(13px, 1.35vw, 15px)`, desc 12px.
- Icons: the poster's glyphs are engraved plates — outline + hatch/dot fill,
  ~2px strokes, visibly hand-drawn (the ONNX mark is a squared spark, the HF
  mark is a high-contrast emoji-like face, the cache is a squat cylinder with
  rim hatching). Replacing the six clean vectors with duotone/hatched SVGs is
  the last visual gap in this column.

### 3.5 Pipeline notes

Poster labels are ~16px sans bold with `.07em` tracking, numbers 12px mono grey,
descriptions 12px mono grey at `line-height:1.5`, and the leader is a
**three-segment step** (right → down → right) drawn in light grey, not a black
two-segment elbow:

```css
.note::before{                     /* plate edge → step */
  left: calc(-1 * var(--leader) - 14px);
  top: calc(50% - 14px);
  width: 14px; height: 1px; background: rgba(11,11,11,.45);
}
.note::after{
  left: calc(-1 * var(--leader) - 14px);
  top: calc(50% - 14px);
  width: 1px; height: 28px; background: rgba(11,11,11,.45);
}
```

Note vertical positions in the poster: label baselines at 613 / 728 / 861 / 984,
i.e. not an even 23.24 % ladder — set them from the plate ports so the notes
track the (now shorter) stack: `top: 8% / 31% / 57% / 79%` after the C1 resize.

### 3.6 Footer

`© 2024` → **`© 2026`** (the poster prints 2024, but the maintainer confirmed
2026 is correct and `LICENSE` already reads `Copyright (c) 2026 elcuervo`;
the poster is out of date here and must not be copied back). Keep the 60px
band, 1px rule and the left/right mono pair.

---

## C4 — Pipeline artwork

1. **INFERENCE plate is a light→black grid ramp.** The poster's second plate is
   paper-coloured and faintly gridded at the front-left corner and becomes solid
   black with *light* grid lines across the back-right two thirds, with a
   photographic/stipple grain through the transition. Today it is a uniformly
   light plate with a black grid at `opacity:.7` — effectively inverted.
   Implement as two stacked `<pattern>` fills plus a linear `mask-image` ramp
   (light grid over paper → `#0B0B0B` fill with `#F3F0E8` lattice).
2. **Cubes.** ~40 blocks instead of 27, taller extrusion, three sizes, clustered
   toward the middle-right of the plate, with a few orange. Current blocks read
   as flat because the side faces are only 11 units tall against a 26-unit top.
3. **Ports** stay as they are (orange dots on the plate centre) — they match.
4. **Plate labels** (`TEXT`, `MODEL`, `VECTORS`, `REDIS`) already ride the plane
   correctly; keep the same `matrix()` shear but update it to the new 0.30 ratio.

---

## C5 — Texture

The poster's giant wordmark is **solid black with an irregular photocopy
speckle** — a handful of sub-pixel paper-coloured flecks, no visible lattice.
The current `.wordmark` paints a regular `3.5px` radial-gradient dot screen via
`background-clip:text`, which at 1086px reads as a coarse halftone and is the
single most obvious mismatch on the page.

Replace with an irregular mask (fractal noise + threshold), e.g.

```css
.wordmark{
  color: var(--fg);
  -webkit-mask-image: url("data:image/svg+xml,…feTurbulence baseFrequency='.9'
    numOctaves='2' + feColorMatrix saturate 0 + feComponentTransfer to a hard
    threshold…");
  mask-size: 180px 180px;
}
```

tuned so roughly 0.5–1 % of the ink is knocked out. Then remove the
`@supports ((-webkit-background-clip:text))` block entirely.

Also soften `body::after` grain: the poster's paper noise is finer
(`baseFrequency` ≈ 1.1–1.3, tile ≈ 200px, `opacity: .045`).

Optional detail visible in the poster: a **faint dashed diagonal** crossing the
wordmark from lower-left to upper-right (`stroke-dasharray: 6 9`,
`rgba(11,11,11,.25)`), reading as a fold/registration line.

---

## C6 — Landscape & route

1. **Knockout, not a plate.** Supply `assets/img/mountain-cutout.png` (or an SVG
   `mask-image` traced from the ridge) so the photo sits as a silhouette on the
   paper. Remove `object-fit: cover` on a rectangle, the `grayscale/contrast`
   filter stays, and delete `.landscape__halftone` plus every wash gradient.
2. **Remove the credit and caption.** There is no `IMAGE — UNSPLASH` or
   `PLATE 01 · SIGNAL PATH / TOPOGRAPHIC ROUTE` in the poster. If attribution is
   required, move it to the footer in 9px mono.
3. **Route.** Collapse `.route--ghost` and `.route__marks` — the poster draws one
   continuous 4px orange polyline with ~6 vertices from the peak at (700, 1130)
   down-left to (390, 1400) and down-right to (1086, 1395), i.e. the vertical
   spine *becomes* the ridge line at the peak rather than a second path.
4. **Annotations** move into `.landscape` as described in C0, at 11.5px mono in
   ink (not muted), each with a 33 × 2 rule, plus the `+` registration mark at
   (962–1000, 1090–1128) — a 1px-stroke 38px plus sign.

---

## C7 — Motion

The poster is a still, so this is judgement, not measurement — but note that
C1.3 (signal above the plates) and C4.1 (the ramped INFERENCE plate) are much
easier to animate if the signal is the top layer: the current behaviour of the
line hiding behind the slabs reads as a bug once the plates are opaque. Keep the
existing sequencing, `prefers-reduced-motion` handling and the deterministic
reveal strategy; just move the spine and let the port dots ride on top.

---

## Verification checklist

After the changes, at a **1086 px viewport** — measured with
`getBoundingClientRect()` on the live page:

| | target (poster) | achieved |
|---|---|---|
| `scrollHeight` | ≈ 1450 | **1439** |
| spine centre | 700 | **696** |
| plate ports | 634 / 736 / 872 / 1001 | **614 / 744 / 874 / 1004** |
| note numbers x | 902 | **894** |
| hero CTA box | 225 × 58 | **226 × 63** |
| feature row pitch | 59 | **59** |
| landscape top | ≈ 1130 | **1096** |
| footer rule | 1388 | **1386** |

- [x] `document.documentElement.scrollHeight` ≈ **1450** (was 1963)
- [x] spine centre on x **700** (±8) at every section it crosses
- [x] wordmark bled off both edges, note inside the `b` counter
- [x] claim tracked at ~0 (was `-.03em`), dead `font-stretch` removed
- [x] hero CTA box **226 × 63** at (35, 833), 5px offset shadow
- [x] feature name baselines ≈ **988 … 1310**, pitch **59**
- [x] plate half-width **197**, plate pitch **130**, four ports on the spine
- [x] mountain is a silhouette on paper: no rectangle edge, no caption, no credit
- [x] footer rule at **1386**, "© 2024"
- [x] wordmark reads as solid black at arm's length (no visible dot lattice)
- [x] the five extras (caption, credit, halftone plate, ghost routes, orange em
      dash) are gone; the terrain keeps a faint 3px print screen instead

### What changed shape during implementation

- **The terrain is not a CSS mask.** Chrome will not composite
  `mask-image` applied before the large async photograph finishes loading,
  which renders the terrain invisible with no console signal. It is an
  inline SVG `<clipPath>` instead — same skyline, no ordering hazard.
- **The terrain frame is `mountain.jpg` cropped to 500,550 → 2600,1420**
  (aspect 2.412, matching the block) so the massif's dark body fills the
  frame and the ridge lands below the annotation band.
- **Archivo Black was *not* adopted for the claim** — measured, it is ~14%
  wider than Archivo 700 at the same size, which would overshoot the
  poster's 367px "embedding server".
- **`font-stretch` is dead code everywhere**: `archivo-var-latin.woff2` has
  no `wdth` axis (62.5% / 100% / 125% all measure identically). The `wght`
  axis does work.

---

# Addendum — second pass (same poster, objective measurements)

The poster is `/tmp/rootshell-uploads/drop-20260912-171157.png` (byte-identical
to the file the first pass used). Method changed: instead of eyeballing a grid
overlay, both images are loaded into a `<canvas>` and probed pixel-wise, and the
site's DOM is measured with `getBoundingClientRect()`. The key trick is
**same-string comparison**: the poster and the site contain the same words, so
the ratio of their ink widths gives the size/tracking error directly, with no
character counting.

## First-pass numbers that were wrong

| item | first pass said | poster actually |
|---|---|---|
| masthead nav | DOCS at x379, gaps 29px, block centre 46.3% | DOCS at x**456**, gaps **41px**, centre **55.6%** |
| masthead CTA | x742–875 (133 wide), inset 190px from the right | x**891–1049** (158 wide), flush to the right gutter |
| masthead tagline | mono ~10px, tracking .1em | 11px, tracking **.2em**, pitch 14 |
| brand `emb` | ~30px display | 34px (ink 68 × 25) |
| hero meta left | tracking .07em | tracking **.29em** (advance 18.3px at 20.5px) |
| hero meta right | 26px | ~17px (advance 11.8px) |
| features | icon x45, names 14px | icon x**49** (drawn ~30px), names ~13.5px, descs ~10px |
| note labels | ~16px | ~**12.5px** (INPUT = 40px of ink, not 53) |
| footer | tracking .13em | tracking ~.01em (advance 6.06px at 10px) |

## What changed in this pass

**Masthead** — brand 3.15vw, tagline 11px/.2em/1.28, nav gap 3.34vw with the
block anchored to the right end of its column (10.3vw of air before the CTA),
CTA flush to the gutter at ~160 × 45 with sentence-case bold ~16px, and the
whole cluster nudged 6px below the optical centre.

**Hero meta band** — left block 20.5px/.29em/1.27, middle 10.4px/.13em/1.45
hung 21px above it, right block 17.3px/.08em/1.55 hung 25px above it. This is
what produces the poster's "wide technical annotation + tiny code block"
contrast; the site previously had all three at roughly one tracking.

**Prose column** — the claim sits 13px below the pipeline's top edge, the
sub-paragraph runs at 1.45 leading, and the column's right padding shrank so
the CTA row (button + `View on GitHub`) fits on one line as it does on the
poster (it was silently wrapping to two).

**Feature list** — 9px indent (icon ink lands on x49), 22px icon gap, 30px
icons, 55.4px row pitch, `.1em` head tracking, 83px head rule. The block starts
at y983 so the head's ink lands on the poster's 990.

**Pipeline notes** — labels 12.5px, descriptions 10.5px at 1.3 leading, and the
stepped leader actually draws now: `--leader` was defined as a *colour* and used
in `calc(-1 * var(--leader))`, so every leader was an invalid declaration and
the pseudo-elements fell back to their static position. `--leader-c` (colour)
and `--lead` (length) are now separate; the leader runs from the plate's edge →
right → down → right into each note.

**Pipeline artwork** (regenerated from `tools/gen-isometric.py`) — plate
half-width 197 → **182**, dimetric ratio .3046 → **.326**, thickness 22 → **28**,
viewBox 394 × 548 → **364 × 553**; port ellipses deleted (the poster's stack has
no port dots — the signal line is the only orange); cube field 42 → **74** blocks
at 0.36–0.82 scale; the INFERENCE ramp pushed further toward black; the light
lattice on SERVE dimmed to .45.

**Wordmark** — size 53.9vw (x-height 323px, the poster's), `scaleX(.93)` instead
of `.98` so the `em` ink ends at 735 rather than 800, and one extra pull of
`-.06em` between `e` and `m` only (the poster's `e`→`m` gap is 35px where
Inter's is 70, while its `m`→`b` gap of 13px matches Inter's exactly). The `b`
counter annotation moved into the counter (x87.6%, bottom 27%) and its leading
was tightened to 1.2 so the five lines stay inside the oval — at 1.5 the last
two lines were clipped by the bowl.

**Landscape** — crop window 1900 × 790 at (140, 590) so the massif sits under
the signal spine; contrast raised to `brightness(.68) contrast(3)` (region means
now match the poster's within ~5/255); the route is a vertex-by-vertex trace of
the poster's orange polyline, forking from the spine at the summit; annotations
moved to `top: 3.7vw`, line-height 1.58; the registration mark back to the
gutter. The print-screen `<rect>` is emitted by the generator again (the first
pass added it by hand, and it was dropped when the block was regenerated).

**Tablet (≤900px)** — the signal's long tail (`y1=-520`) ran up through the
feature list once the pipeline stacks below the prose; `.pipeline__svg` is
clipped at that breakpoint so the spine starts at the INPUT plate.

## Verified after the pass (1086px viewport)

| | poster | site |
|---|---|---|
| `scrollHeight` | 1448 | **1449** |
| masthead brand ink | x42–108 | x41–110 |
| nav words | 456 / 523 / 605 / 688 | 455 / 522 / 604 / 687 |
| CTA box | 891–1048 × 17–61 | 885–1044 × 16–60 |
| meta-left rows | 134, 160 | 135, 161 |
| meta-mid rows | 113, 128, 143, 158 | 112, 127, 143, 158 |
| claim ink | 601–766 | 601–761 |
| hero button | 44–268 × 875–930 | 41–263 × 877–932 |
| `View on GitHub` | x317–444 | x317–437 |
| feature head ink | 990 | 991 |
| note 1 / note 4 label ink | 607 / 976 | 607 / 976 |
| terrain ink | 1142–1394 | ~1145–1394 |
| footer band / rule | 1394–1448 | 1394–1447 |

## Intentional exceptions

- **The poster's mountain is a different photograph.** Its silhouette cannot be
  reproduced from `assets/img/mountain.jpg` (a pixel search over crop windows
  cannot match the poster's ridge within ~50px anywhere). The site keeps its
  own placeholder asset — already flagged in `PRODUCT.md` as pending licensing —
  with the poster's silhouette height, darkness, annotation band and route.
- **The `b` counter is smaller than the poster's** (Inter's bowl opening is
  narrower than the logotype's), so the counter annotation runs at 10.5px there
  while the poster's is ~11.7px. At 12px+ the oval clips the first and last
  characters of a line.
- **Small mono still runs below the 12px floor** in `PRODUCT.md` — feature
  descriptions 10.2px, note descriptions 10.5px, footer 10px — because the
  poster sets them at 10–10.5px. The muted grey measures 4.74:1 on the cream
  ground, so this is a size deviation, not a contrast one. Flagged, not hidden.
- **Inter remains the display face.** The detector flags it as overused, but
  its stem-to-x-height ratio (0.36) matches the poster's logotype where Archivo
  Black's does not; the poster's `e` terminal is the one letterform that differs.
- **Wide tracking (`.2em`–`.29em`) on mono annotations** is flagged as a
  body-text antipattern; here it *is* the poster's treatment, restricted to
  short uppercase metadata blocks.

---

# Addendum — third pass (12 September 2026)

Compared the current implementation with the newly supplied 1086 × 1448
reference using local Chrome screenshots. The earlier claim that the font
wordmark was the closest available match left a substantial visible gap.

## Changes

- Replaced the giant font lockup with three native SVG outlines following the
  reference: horizontal `e` terminal, narrow `m` arches, taller `b`, and the
  larger oval counter. Its visible bounds now run from y87 to y567, with the
  x-height beginning at y218. The counter caption fits at approximately 11.5px.
  Added restrained ink grain and the diagonal print crease.
- Corrected the CTA and feature typography, sentence-case GitHub underline,
  metadata treatment, quiet footer rule, and feature icons. Kept the confirmed
  **2026** copyright year. The main button starts at y875.
- Regenerated the pipeline with 119-unit pitch, 24-unit thickness, 46 input
  marks, and 48 smaller, taller blocks. Light slab edges, black SERVE side
  faces, surface grain, larger plate lettering and a darker INFERENCE ramp
  bring the diagram closer to the reference. Tall return brackets replace
  the small elbows, and note text sits to the right of the plate edge.
- Widened the existing mountain crop to bleed left beneath the feature list;
  the knockout edge now follows an irregular slope. Moved the registration
  mark and left-aligned the right annotation.
- Fixed the route's scroll calculation. Previously its denominator was almost
  an entire viewport, so neither branch finished drawing at the page bottom.
  It now finishes when the terrain has entered view.
- Fixed overlapping mobile metadata, separated the technical values, kept
  terrain annotations above the image, and aligned the tablet route with the
  stacked diagram. Preserved reduced motion and the static no-JavaScript view.

## Measurements and limits

At 1086px wide: page height **1449px**, wordmark top **86.8px**, wordmark bottom
**566.8px**, main CTA top **875px**, footer rule **1394px**. The reference frame
is 1448px high. Desktop and mobile screenshots were inspected directly.

The mountain is still a different photograph, and the pipeline remains SVG
artwork rather than the reference's photographic rendering. These are the
main remaining visual differences; this is not a pixel-identical replica.

Browser checks cover 320, 390, 768, 980, 1086, 1440 and 1920px widths for
horizontal overflow, heading overflow, font loading, asset failures, duplicate
IDs, six features and four stages. Motion checks cover completed routes and
reveals at the bottom of the page, plus note hover and keyboard focus.

---

# Addendum — fourth pass (continued visual refinement)

- Replaced the snowy placeholder with `assets/img/terrain-v2.png`, generated
  from the supplied poster as a visual reference. The new dry, craggy massif
  has the broader foothills, twin peak, dark left face and right saddle of
  the reference. Its light background blends into the paper; it is not an
  alpha-transparent PNG. Removing the landscape's isolated stacking context
  is necessary for that blend to work correctly. The old photograph remains
  in the repository but is no longer used by the page.
- Refined the orange ridge route, including its lower-left endpoint at the
  footer and the short stepped bends on the right shoulder.
- Added ruled slab edges, stronger print texture, cube contact shadows and
  lifted wire lattices. Corrected plate labels to rise toward the right along
  the plate plane. The signal now disappears behind each plate's front edge,
  matching the depth visible in the reference; this supersedes C1.3's earlier
  assumption that the entire line should remain visible over every surface.
- Simplified `tools/gen-isometric.py` to own only the pipeline and added
  `--write` for deterministic updates to `index.html`. It no longer emits
  obsolete terrain clip paths or requires manual copy/paste.

The 1086px composition still measures 1449px high, with the footer at y1394.
The seven-width browser checks, keyboard focus, note hover, completed route
drawing and JavaScript syntax checks pass. The generated terrain and vector
pipeline remain interpretations of the reference, rather than its original
source assets. Generation prompts are in `assets/img/terrain-v2.md`.

---

# Addendum — fifth pass (mobile, compositing, one cut-out)

Three reported defects: the terrain flashing its own off-white ground before
settling, animation/z-order that could not be trusted, and a mobile scaffold
that stopped short. One change fixes the first two and most of the third.

## The terrain is now a real cut-out

`terrain-v2.png` was generated as an RGB image with a flat warm ground, not
alpha, and the page faked the cut-out with
`mix-blend-mode: multiply` + `filter: grayscale(1) brightness(1.08)` + a
polygon `clip-path`. `multiply` blends against its *backdrop*: where the image
sits in a stacking context whose backdrop is empty there is nothing to
darken, and the source paints as-is — the off-white ground, i.e. an opaque
rectangle. That is the flash. It is also the exact failure the notes in the
README were already excusing ("the landscape must not establish an isolated
stacking context"), which is a fragility, not a design.

`tools/gen-terrain-matte.py` (stdlib only, via `tools/png_lib.py`) now bakes
the same grade into the pixels and writes `terrain-matte.png`:

```
alpha = 1 - L / 235      rgb = 0 (black)      L = .213r + .715g + .072b
```

Black at that alpha over the paper *is* `multiply`: `paper·(1−a) = paper·L/235`.
235 is the ground's own level (1st percentile of the sky sample), so the ground
lands on alpha **0** and no compositor state can bring it back.

Measured, not asserted:

| check | result |
|---|---|
| matte vs the old multiply chain | mean **0.33/255**, peak **1.6/255** |
| sky pixels above alpha 8 | **37 of 797,399** (0.0046%, all isolated single px) |
| rock coverage | 34.6% of the frame |
| file size | **0.80 MB**, 37% of the 2.29 MB source |

`clip-path`, `filter` and `mix-blend-mode` are all gone from
`.landscape__photo`. That also removes a 78vw-wide filtered, blended layer
from the compositor, which is the expensive kind on a phone.

## Z-order is now stated, not inherited

`.landscape` owns a stacking context (`isolation: isolate`) and each layer
carries its level: cut-out 0, route 1, annotations and the registration mark
2. The section also takes `z-index: 1` so the terrain always paints above the
pipeline it overlaps by 11px, instead of depending on DOM order.

## The route is glued to the rock

The route used to live in a `630 × 272` viewBox stretched across the landscape
band while the artwork lived in its own box with its own offsets. They drifted
against each other as the viewport changed, and the `left: -29.1%` mobile
override was where that drift had been patched by hand.

Now the cut-out and the route share one box — `.landscape__art` — whose
`aspect-ratio` is the artwork's (`2172 / 770.2`); the polyline is authored in
the artwork's own pixels. Two consequences, both measurable:

- the box is 78vw wide with a 1.8vw bleed, and the fork at `x1124.3/2172`
  lands on the signal axis at **every** width. Worst error over 1001px → 3440px
  is **0.09px** (it is exact below the shell's 1720px cap, where both the art
  box and the spine are linear in the viewport);
- `--band` is 27.66vw, which is 78vw of artwork at `2172 : 770.2`. Band height
  and artwork height now agree to **0.02px** at every width, so the massif is
  no longer squeezed: it was 4% off below 1410px and **27% off at 1720px**,
  because a `375px` cap froze the height while `78vw` kept growing. Both the
  band and the art width now cap at the shell (`--maxw`), which also stops the
  massif sliding out from under the spine on a very wide display.

The route's stem (the spine's continuation, `y0 → y243.5`) and its fork
therefore stay collinear with `.sig` at every width, and the two orange
strokes now match in weight (5.17px vs 5.16px at 1440px) because
`vector-effect: non-scaling-stroke` was replaced by `stroke-width: 10` in a
viewBox whose scale is uniform.

## Motion

- **The slab stagger no longer leaks into the hover.** `transition-delay` is
  per element, not per state, so `transform: translateY(-6px)` on `.is-hot`
  was waiting out that slab's entrance delay — up to 0.51s on SERVE. The
  entrance now rides `transform` and the lift rides `translate`, with a
  per-property delay list (`.17s, .17s, 0s`), so hovering is immediate in both
  directions. `translate` on an SVG group is the progressive part: where it is
  unsupported the lift is merely absent.
- **The slab delays are keyed by `data-slab` name**, not `:nth-of-type`, so
  regenerating the pipeline cannot silently reshuffle the sequence.
- **The route always finishes.** Progress is measured against the band at
  `86%` of its travel; at the full band height that branch was reaching the
  page bottom at 97% on a 1440 × 900 window, leaving the last segment undrawn.
- **Reveals ripple.** `--i` carries each feature's and each stage's place in
  its list, so a group resolves as a sequence rather than one block flip.
  Reduced motion still collapses the whole cascade to a single frame.

## Mobile

- **Phones keep the isometric stack.** It was `display: none` below 640px —
  the page's central argument, deleted at the width where a reader has the
  most attention for it. It now runs full width under the prose, and the four
  stages follow as a ruled list.
- **The accent left border is gone** (`border-left: 3px solid var(--accent)` on
  `.pipeline__notes`), which the review rules call out by name. The list is
  closed by a hairline rule on the container instead.
- **The terrain is aspect-true on phones and tablets** rather than
  `object-fit: fill` over `110vw × 62%` (2.38:1 against the artwork's 3.00:1).
  Both the ≤1000px and ≤640px overrides are now one rule each, and the route
  follows because it shares the box.
- **Notches.** `--pad-l` / `--pad-r` fold in `env(safe-area-inset-*)`, and
  everything that cancels a gutter to reach the sheet's edge — the terrain,
  the annotation band, the mobile menu — cancels those instead. The masthead
  also carries the top inset, for a standalone window.
- **Short landscape windows** scale the wordmark to the available height
  (`min(44.2vw, 56vh)`) instead of cropping it with `overflow: hidden` and a
  `-13vw` nudge.

## What this changes in the composition

At 1086px the band goes from 289px (26.6vw) to 300.4px (27.66vw): the
landscape top stays at 1096, the footer rule moves 1386 → **1397**, page height
1449 → **≈1460**. Those three figures are arithmetic on the recorded
measurements, not a fresh browser pass.

## Verification

This pass was first verified by measurement rather than by looking — the matte
against the pipeline it replaces (pixel diff), the sky against transparency
(alpha histogram over the whole region above the ridge), the layout against
itself (the spine/fork and band/art agreement recomputed at 13 widths from
1001px to 3440px), and the files against the detector, which parses both
cleanly.

It has since been looked at, in Chromium, once the browser tooling was in
place (`agent-browser` from nixpkgs plus Chrome for Testing, or the Playwright
Chromium already on this machine via `AGENT_BROWSER_EXECUTABLE_PATH`).

**Desktop, 1280px.** The cut-out shows no rectangle and no flash; the route
forks exactly where the spine lands and rides the ridge down both flanks; the
annotations and the registration mark sit in the sky above the rock.
`scrollWidth` equals the viewport, so nothing bleeds sideways.

**Phone, 390px.** `scrollWidth 390` against a 390 viewport — no horizontal
overflow. The isometric stack is present and full width, the spine reads
through each plate, the four stages follow as a ruled list with hairline
separators and no accent bar, and the terrain is aspect-true with the route on
the ridge.

One defect the browser found and measurement could not: on a phone the
spine's tail ran 47 units past the last plate into empty space, because in the
stacked layout the terrain it is heading for is a screen and a half below. The
diagram now ends at the plate, clipped by its container
(`.pipeline__svg{margin-bottom:-12.9%}` — the empty tail is 12.9% of the
artwork's width). Page height at 390px went 2708 → 2574.

Two things to know about the harness. The pi wrapper **strips `--viewport`**
before spawning upstream, so a width check cannot be driven from the wrapper's
launch flags; the 390px render above was taken by injecting a
`position:fixed;width:390px` iframe and screenshotting the outer document,
where the media queries evaluate against the iframe's own viewport. And
`just website-shot` passes `--viewport` to the CLI directly, for shells where
the wrapper is not in the way — that flag is upstream's, not the wrapper's, so
it is exercised by the maintainer rather than by this pass.

---

## Pass 6 — depth order, the type floor, and three data branches

This pass came out of a critique of the built page (`critique` over
`website/index.html`), and it is the first one driven by measurements taken
against the *page* rather than against the poster. New reference numbers, all
at Chromium 1187 / DPR 1 over `python3 -m http.server`:

**1. The plates were painted back-to-front-wrong.** With the camera ~19° above
the horizon the *highest* plate is the nearest one, so INPUT has to paint
last. Painted INPUT-first, each lower plate's top face cuts into the front
skirt of the plate above it over a **73 × 24 unit wedge** centred on the
spine; it is only invisible because the signal line covers the middle of the
seam. The four `<g class="slab">` groups are now emitted SERVE →
EMBEDDING → INFERENCE → INPUT, each carries `data-depth` (1 farthest,
4 nearest), and `gen-isometric.py` reorders them so `--write` cannot revert
it. Verified at 2.3× zoom: the upper plate's skirt now hangs over the plate
below at every seam. The activation stagger was already keyed by name, so the
sequence read out of the document unchanged.

**2. The committed type floor was false.** Measured computed sizes at the
1086px reference frame had the footer at **10.0px**, `hero__meta-b` and
`rule-head` at **10.5**, `note__desc` at **10.53**, `feat__desc` and the
landscape annotations at **11.08** — every annotation `clamp()` had a 10–11px
minimum, so from 1001px to ~1240px the *minimum* governed. On phones the
smallest was `hero__meta-b` at **11px**. Every annotation clamp minimum is now
12px and the ≤1000px block sets 14px explicitly, which is the floor the brief
and `PRODUCT.md` both committed to. Re-measured: desktop 1086 → 12.0px
minimum, phone 390 → 14.0px minimum.

**3. The accent failed contrast wherever it became ink.** `#FF5A1F` on the
paper is **2.74:1**. It was carrying the `:focus-visible` ring (needs 3:1,
WCAG 1.4.11) and the hover state of the GitHub link and the stage number at
13px (needs 4.5:1, WCAG 1.4.3). A `--accent-ink` token at `#C23D00`
(**4.66:1**) now carries both; `#FF5A1F` stays for surfaces.

**4. The hero entrance was one step of six.** `document.getAnimations()`
returned exactly one animation on load — the spine at 150 + 850ms. The
masthead, both annotation blocks, the wordmark and the prose had none. They
now land 90ms apart on `--e` ordinals (settling at 780ms) with the spine
finishing at 1000ms, inside the brief's 800–1200ms window and collapsed to
the end state under `prefers-reduced-motion`.

**5. The mobile claim lost its authored line breaks.** `.claim span{display:
inline}` below 640px collapsed the poster's four lines into three wrapped
ones. `display: block` is restored and the size retuned to
`clamp(30px, 8.4vw, 44px)`; measured 4 lines at 390px and at 320px.

**6. Reveals could still be left invisible.** The sweep ran on scroll, resize
and one initial frame, so a print pass or a headless full-page capture that
never scrolls rendered the features, the plates and the annotations at
`opacity: 0` — reproduced, not theorised. A `ResizeObserver` on the root now
catches everything else that can move the trigger line (zoom, rotation, a late
font or image), and a `@media print` block collapses the cascade to its end
state. `README.md`'s claim that a full-page capture can never hide content was
corrected rather than left standing.

> **Correction, pass 7.** The `ResizeObserver` closed the *resize* cases, not
the capture case, and the claim above was still too broad. A
`captureBeyondViewport` screenshot neither scrolls nor resizes, so the trigger
line never moves and every reveal below the fold stays at `opacity: 0`.
Reproduced at 390px on a fresh load: **10 of 20 `[data-reveal]` elements**
hidden, including the whole new capability region and the `02–04` stage notes.
Print is genuinely safe (`@media print` collapses the cascade); a screenshot is
not. `just website-shot` now scrolls to the end and back before capturing, and
`README.md` states the limitation instead of denying it.

**7. Three data branches carry the README into the artwork.** Above the fork,
`BLOB OR VALUES`, `HELLO 3` and `1 MS WINDOW` fork off the trunk with margin
annotations; the trunk draws across the whole travel and each branch over the
following 58% of it, 14% apart. Below 1000px the branches and their labels are
dropped together — a phone has no paper beside the massif.

Smaller items in the same pass: the masthead gained its hairline rule on
desktop (it was `transparent` above 1000px, so content vanished under an
opaque sticky edge); the mobile menu's hover colour moved off `--muted` so a
tapped row is not left *lighter* than its rest state; `.pipeline__notes`
carries `role="list"`; the reduced-motion block zeroes `transition-delay` as
well as duration (the durations alone left the 0.17/0.34/0.51s waits in
place); the `main.js` font fallback timer is cleared once `fonts.ready`
resolves instead of re-firing at 1236ms; the note ⇄ plate emphasis is wired in
both directions and answers `pointerdown`; and `main.js` stops scrubbing the
route when a reader turns reduced motion on mid-session.

`DESIGN.md` was corrected where it disagreed with the implementation rather
than the other way round: `--muted` is documented as `#6B6963` (4.82:1, AA)
instead of `#77756F` (4.05:1, which would have failed), the `REDIS` plate
label against the `04 SERVE` note is documented as the brief's own split, and
the phone annotation-list decision is recorded with the arithmetic that forces
it.

---

## Pass 7 — the capability region, and a capture claim that was still false

This pass adds a second spread below the terrain (the capability region: three
ruled entries, the console placeholder, the `emb-top` band) and re-measures the
page around it. Everything below is taken from the built page at Chromium 153 /
DPR 1 over `python3 -m http.server`.

**1. The poster ratio now governs the hero, not the document.** The hero spread
still measures 1464px at 1086 — unchanged, 1 : 1.348 against the poster's
1 : 1.333. With the new region the document measures **1086 × 2486** (1 : 2.289).
The ratio was explicitly released for this work: the addition is a second
spread, not a longer sheet.

**2. Nothing above the capability region moved.** `git diff --numstat` on the
three site files reports **127 / 0** (HTML), **230 / 0** (JS) and **284 / 4**
(CSS). The four CSS deletions are the section index comment and two section
banners being renumbered to make room for `08 capability`; **no rule outside
the new section changed**, so the hero, wordmark, pipeline, terrain and footer
render byte-identically.

**3. The console introduces no new colour, and every value is measured.** It
reuses the SERVE plate's `#111110` top face and `#292823` side face, the plates'
1px `--bg` stroke, and the page's existing tokens as ink. Measured against the
panel's own background:

| Element | Colour | Ratio | Needs |
|---|---|---|---|
| ink (`--bg`) on `#111110` | `#F3F0E8` | **16.59:1** | 4.5:1 |
| dim voice / badge / input border (`--rule`) | `#B8B5AC` | **9.22:1** | 4.5:1 (3:1 non-text) |
| prompt, error prefix, focus ring (`--accent`) | `#FF5A1F` | **6.06:1** | 4.5:1 (3:1 ring) |
| active tab, `--rule` on `#292823` | `#B8B5AC` | **7.20:1** | 4.5:1 |
| `--accent-ink` — *not used here* | `#C23D00` | 3.52:1 | would fail 4.5:1 |

`--accent-ink` is the one token that does not travel to the dark surface: it is
tuned for the paper (4.66:1) and fails as text on `#111110`, so the console
overrides the global focus ring back to `--accent`. This is a scoped,
measured exception, not a change to the accent's role.

**4. The dark surface clears the committed type floor at both references.** The
smallest computed text in the whole region is **12.0px at 1086** and **14.0px at
390** — the same floor `CORRECTIONS.md` pass 6 established. Three labels were
initially below it at 390 (badge 12px, note 13px, `Sample run` 13px) and were
raised. A second pass caught two `wide-tracking` findings the detector attributed
to body copy and removed them by dropping one incidental `.06em` and lowering the
other to the documented `.05em` threshold.

**5. The panel overflowed the shell, and the detector found the padding.** The
first build of the region set `body.scrollWidth` to **1452 against a 1280
viewport** — the emb-top `<pre>` was sizing its grid track to max-content
because the panels container had no explicit shrinkable column. Fixed with
`grid-template-columns: minmax(0, 1fr)`; re-measured at 1280, then at 1440, 1086
and 390: `body.scrollWidth === clientWidth` at every width, and the emb-top
capture fits the shell at 1086 without scrolling (1004 / 1004). The detector's
final run adds **2 `cramped-padding` warnings** and removes **1
`flat-type-hierarchy`** (the new type ladder gave the page the step it lacked).
Both padding warnings were checked and are **false positives**: the flagged bar
has 16.29px horizontal padding, and its children measure **16px** inset from it.

**6. The full-page capture claim was still false — corrected, and the tool
fixed.** See the correction under pass 6. `just website-shot` now scrolls the
page to the end and back before capturing, because a `captureBeyondViewport`
screenshot reproduces only what has already been revealed.

**7. Keyboard, motion and no-JS were verified, not assumed.**

```text
ArrowRight on the tablist   → focus tab-scripts, aria-selected flips,
                              roving tabindex 0/-1, panel labelledby updates
Type + Enter                → echo, then staggered result lines (~250ms)
prefers-reduced-motion      → full transcript in ONE frame, no stagger
unknown command             → -ERR unknown command 'FLUSHALL' + hint
JavaScript disabled         → form hidden, 12-line <noscript> transcript stands in
320–834px controls          → mode 44px, input 47px, RUN 44px
```
