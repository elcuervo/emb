# Corrections — matching the site to the poster reference

> **Status: implemented.** Every item below has been applied to
> `index.html`, `assets/css/styles.css` and `tools/gen-isometric.py`. The
> measured after-state at a 1086px viewport is recorded at the end of this
> file; the numbers below are the *before* measurements that drove the work.

Source of truth: `/tmp/rootshell-uploads/drop-20260911-195139.png`
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

`© 2026` → **`© 2024`** (the poster says 2024). Keep the 60px band, 1px rule and
the left/right mono pair.

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
