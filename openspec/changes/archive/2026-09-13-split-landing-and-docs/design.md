## Context

See `proposal.md` — Why. Constraints that shape the approach:

- The site is static HTML/CSS/JS with no build step, no CDN, and a committed
  promise that `index.html` works over `file://` (`website/README.md`). Any
  second surface inherits that promise.
- `styles.css` is currently page-global: every token is on `:root` (including
  `--fold: 65.31%`, the hero's own derived axis), the breakpoints are shared
  width-only rules, and the paper grain is a fixed `body::after` layer. A `/docs`
  page that simply links the sheet inherits the hero's axis and shell.
- `README.md` (755 lines) already has the reference's structure: Features,
  Install, Quick start, Commands, Custom scripts, Configuration, Operations,
  `emb-top`, Clients. `/docs` reorganises that structure for a page; it does not
  invent content.
- `website/PRODUCT.md` fixes the audience ("engineers who already operate
  Redis") and the no-fabrication list, and commits the type floors (12px
  desktop / 14px mobile) and the contrast rules.
- The page's animation contract is a single enhancement class: an inline script
  sets `html.js`, and only `main.js` adds `is-ready`, `is-live`, and the reveal
  `.in` classes. The styles that hide content for the entrance are gated on
  `html.js`, so the class and the script are a matched pair with no failure path.
- Twelve measured correction passes exist in `CORRECTIONS.md`. The first browser
  measurement of the composed page found a defect every one of them was
  structurally unable to see.

## Goals / Non-Goals

**Goals:**

- The landing page is a poster that earns a click; the documentation surface
  carries the reference.
- The documentation surface is a second page in the same world, not a second
  product, and not a fork of the stylesheet.
- Every claim on either surface is traceable, and every version string is
  derived.
- The composed page is measured on glyph ink, not on box overflow, and survives
  a script failure.

**Non-Goals:**

- **No 3D, WebGL, or WebGPU.** Recorded so it is not re-litigated, with the
  measurements behind each number in
  [`evidence/webgpu-vs-threejs-research.md`](evidence/webgpu-vs-threejs-research.md): `DESIGN.md`
  says "Avoid unnecessary canvas or WebGL" and names 3D glowing spheres as an
  anti-reference; WebGPU is unavailable over `file://` (the site's committed
  entry path) and covers ~84% of visitors at best; three.js is 102–184 KB
  gzipped to draw a fullscreen quad; vgpu's 25 KB budget is a bundler output and
  its headless proof needs a 19 MB native binary. Every in-world use this
  analysis found — a dithered embedding strip, the terrain — reduces to SVG or a
  2D canvas.
- **No re-doing the massif.** The terrain artwork and its route are untouched.
  The matte's re-encode (`cwebp`/`pngquant`, both already in the dev shell) is
  deliberately deferred: it is a byte-level asset change with its own
  verification, and it is not needed to close this change.
- No change to the hero's composition, wordmark, claim, feature list, pipeline
  artwork, or terrain geometry.
- No server, protocol, gem, or Go/Ruby changes; no new dependency, font, or CDN.
- `/docs` does not become a second source of truth for `README.md`'s content.

## Decisions

### D1 — Two surfaces, two modes

`index.html` is **Persuade**; `docs/index.html` is **Read**. The split is not a
way to avoid fixing the landing's copy: the first task is the truth fixes, and
the split follows.

*Alternatives:* keep everything on one page — rejected, because `DESIGN.md`'s own
hierarchy puts implementation details sixth, after the CTA, and the page already
fails its own promise by carrying reference material it cannot keep true; make
`/docs` a link to the repository README — rejected, because the reference is
already written and the site's job is to be the hosted form of it.

### D2 — What stays on the landing

Exactly four things beyond the poster, each earning its place as a pre-click
decision:

| Keeps | Why it cannot move |
|---|---|
| One copy-paste install line | A convinced reader must be able to act without leaving the site |
| One REPL proof (`redis-cli EMB minilm "hello world"` → bytes) | It is the whole positioning in one command |
| Pre-1.0 status, license, platform list | A reader must know the risk before installing |
| One measured figure from `BENCHMARK.md` + its reproduction command | "Fast" is the thesis and the page currently carries no number |

Everything else — command tables, the YAML block, reply-format prose beyond one
line, the `emb-top` figures, version strings, per-model introspection — exists on
`/docs` only. The landing's `facts` lists stay at four entries each.

*Alternatives:* a "features" page between the two — rejected as a third surface
with no job; putting the install line in the footer — rejected, it is below the
fold on every viewport.

### D3 — `/docs` inherits one world, and does not need a stylesheet split

*(Revised during implementation — see tasks 5.1.)* `docs/index.html` links the
shared `styles.css` plus a page-specific `docs.css`. The rule for anything shared
is: **the value lives in the shared sheet's `:root`; `docs.css` never redefines a
token.** `/docs` carries no axis: `--fold` and the spine primitives stay with the
hero, and `/docs` has no markup that could match them.

The original plan was to extract the shared layers into a third file first,
because `styles.css` puts every token on `:root` and the hero's derived axis with
them. Measurement made that unnecessary: the hero's geometry is *selector-scoped*
and not global. The only element-level rules in the file are `:root`, the `*`
reset, `html`, `body` and `body::after`, and none of them references `--fold` or
the spine; every `var(--fold)` use sits inside `.spine`, `.block__body`,
`.topviz`, `.terrain__art` or `.note`. A docs page with none of that markup
inherits the world and none of the composition, with no file moved and no
measured value put at risk in a 1,700-line stylesheet that twelve correction
passes were measured against.

*Alternatives:* extract the shared layers anyway — rejected as churn that cannot
pay for itself once the selectors are checked, and it would invalidate every
number in `CORRECTIONS.md` for no behavioural gain; copy `styles.css` and delete
what `/docs` does not use — still rejected: the floors are per-selector clamps
rather than tokens, so a hand-copy silently drops minima (the failure mode passes
6 and 11 each had to re-fix) and the two sheets would drift at every breakpoint;
extract a build step — rejected by the no-build constraint.

*Accepted cost:* the docs page downloads the hero's unused CSS (about 30 KB, far
less gzipped) instead of a hand-split subset. It cannot desynchronise, which is
the failure mode the split was meant to prevent.

### D4 — The truth defects are removed, not relocated

`images` is deleted from the hero sub and the INPUT stage note, because
`examples/scripts/siglip2.lua` states that the image branch is not populated and
`pixel_values` is fed as zeros. `/docs` documents that honestly. "Production
ready." becomes the pre-1.0 status. The `emb-top` version string is generated
from `VERSION`.

*Alternatives:* keep the claim and document the caveat only in `/docs` —
rejected: it is a false claim about the product's input surface, and moving a
false claim is laundering it; keep `Production ready.` — rejected: `VERSION` is
`0.4.0.pre4` and `PRODUCT.md` states interfaces may still move.

### D5 — Ink is measured, not boxes

The note-clipping defect is fixed by containing the text inside the sheet while
keeping the leader geometry: at `min-width: 1001px` the note's right offset goes
to `0` and the 10px is reclaimed from the note's own number column, so the
description's measure is unchanged and the leader's anchor does not move.

The regression assertion is two-part and both halves matter:

1. **Box half** — no `.note` box may have `right > documentElement.clientWidth`.
   This fails at *every* width in 1001–1739 today.
2. **Ink half** — the rightmost and leftmost rect of every text run (measured
   with `Range.getClientRects()` over text nodes) must lie inside the viewport.

Checked at 1001, 1024, 1070, 1071, 1086, 1100, 1150, 1151, 1200, 1300, 1340,
1440, 1720, 1721, 1740, 1920 — the measured band edges, the design's reference
frame, the breakpoint, and the shell-cap handoff.

**Why `scrollWidth` cannot substitute:** `overflow-x: clip` creates no scroll
container and removes the clipped content from the scrollable overflow region,
so `scrollWidth === clientWidth` holds whether or not glyphs are being cut. That
comparison was the pass 8–11 check, and it is structurally blind to this defect.

*Alternatives:* `right: 0` alone — rejected, it costs 10px of measure and
re-wraps the longest description to a third line in 1070–1150, which is the
regression the note's current geometry was written to prevent; moving the notes'
left edge instead — rejected, it moves the `01–04` numbers off the poster's
measured position.

### D6 — Three failure paths get a contract

1. **Ink containment** (D5).
2. **Script failure.** `html.js` is set inline and unconditionally, while
   `is-ready`/`is-live`/`.in` come only from `main.js`. If the script never runs,
   the four plates, the spine, and the four notes stay at `opacity: 0` forever.
   The guard is on the script tag: a load error removes `js`, which is the only
   state the hiding styles are gated on. This is the one failure mode on the page
   where content is *invisible* rather than merely unanimated.
3. **Print.** The print block resets the reveal cascade but leaves the dark
   surfaces with `color: var(--bg)`, which prints near-white on white when
   background graphics are off. The fix is `print-color-adjust` on the dark
   grounds.

### D7 — `/docs` loads no JavaScript at all

*(Revised during implementation — see tasks 6.4.)* The plan was for `/docs` to
reuse `main.js` unchanged, since every block in it is feature-guarded. Building
the surface showed it needs none of it: the docs page has no pipeline, no
terrain, no `[data-reveal]` section, and no console, so `main.js` would be a
download whose every branch no-ops. The page ships with no `<script>` element.

That is also the strongest available form of the two promises this surface has to
keep: there is nothing to fail, nothing to gate content behind, and nothing that
needs a build. The `html.js` class is simply never set, so the styles that hide
content for the landing's entrance cannot apply.

*Alternatives:* reuse `main.js` for future interactivity — rejected as paying now
for a maybe; add a disclosure menu for the in-page navigation — rejected, the
contents list is a plain `<ol>` of anchors and needs no JS.

### D8 — Sequencing against `recompose-site-spine`

`recompose-site-spine` is 33/33 but unarchived, so its `product-site` delta has
not been synced into `openspec/specs/product-site/spec.md`, which still carries
the superseded "one merged capability block below the terrain" requirement. This
change's `product-site` delta is authored against the spec as it will read after
that sync. **Order: archive or sync `recompose-site-spine` first; archive this
change second.** Archiving this change against the unsynced spec would leave the
main spec describing a composition that no longer exists.

### D9 — Record the measurement trap

Element-scoped screenshots of the pipeline (`screenshot .pipeline__stack` at a
900px-tall viewport) paint only the first plate and leave the rest blank,
because Chrome's `captureBeyondViewport` does not paint SVG-filtered or masked
content outside the viewport. The diagram is correct; the capture is not. This
belongs in `CORRECTIONS.md` so a future pass does not "fix" a working diagram,
and so no check is ever gated on an element-clipped capture again.

## Risks / Trade-offs

- **[The stylesheet split touches 1,667 lines of measured CSS, and every number
  in `CORRECTIONS.md` was measured against the current file]** → Do the split
  mechanically (move, do not rewrite), change no declaration's value, and
  re-verify the eleven recorded invariants (spine alignment, section gaps,
  scroll width, type floors, contrast) before and after with a cache-busted
  stylesheet.
- **[The split's honest cost is paid before any docs prose is written]** →
  Sequence the tasks so the extraction and its re-verification are their own
  step; do not interleave prose with geometry.
- **[`/docs` becomes a fourth copy of the same facts (`README.md`,
  `website/README.md`, the landing's `facts` lists, `/docs`)]** → `README.md`
  remains the source of truth; `/docs` is explicitly the hosted form; the landing
  is capped at four facts per list and one of each thing in D2; the version
  strings are the one class of fact that must be derived rather than authored
  twice.
- **[A second surface grows its own visual language]** → `/docs` links the shared
  sheet and carries page rules only; the excluded-device list already in
  `product-site` governs it; no new token may be introduced by `docs.css`.
- **[The `--fold` extraction breaks the axis]** → `--fold` and the spine
  primitives stay with the hero rather than moving to the shared sheet; the
  assertion is the existing one (every spine and the terrain fork agree within
  1px at seven widths).
- **[Rearranging copy breaks the measured 1086 frame]** → The hero's composition
  is fixed; only copy inside existing slots changes, and the pre/post measurement
  set in tasks 1 and 6 covers it.

## Open Questions

- Which single `BENCHMARK.md` figure the landing may claim, and whether it is
  quoted in the hero or in the protocol block. The spec requires provenance and
  the reproduction command; the placement is a design choice that does not change
  the task breakdown.
- Whether the install line lives under the action row or replaces the
  second action. Both satisfy "above the fold, without leaving the site".
