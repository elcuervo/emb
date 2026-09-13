## Context

See `proposal.md` — Why. Constraints that shape the approach:

- The page is static HTML/CSS/JS with no build step. Highlighting is authored
  into the markup; no highlighter, no CDN, no dependency.
- `CORRECTIONS.md` records that the spine currently arrives from the pipeline
  SVG: one `<line class="sig">` at `x=182/364` of the plate column, `pathLength=1`,
  masked by `#signal-depth` so it is hidden where it would meet a plate's front
  edge. The terrain route continues it at `x=1124.3/2172` of the art box.
- `.pipeline{margin-left: calc(40.7% - var(--plate)/2)}` inside
  `.hero__pipeline` (column 2 of `.hero__body`, which has no `gap`) means the
  spine's x is exactly `41.5% + 40.7% × 58.5%` of the shell content box —
  `40.7% - plate/2 + plate/2`. The `--plate` clamp cancels, so the axis is
  width-independent. That is the derivation `--fold` formalises.
- `--art-w: min(78vw, .78 × --maxw)` and `--band: clamp(210px, 27.66vw, .2766 × --maxw)`
  are proportional to `--art-w` (the artwork is `2172 × 770.2`), so the band
  height equals the art height in the unclamped range and diverges at the
  clamps.
- `main.js` drives reveals by element position and draws the route from
  `stroke-dashoffset` over the band's travel (`rect.height * 0.86`). Moving the
  band changes that window.
- The archived change left the console, its transcripts, its states, and its
  `window.embConsole.exec` seam in place. This change must not disturb them.

## Goals / Non-Goals

**Goals:**

- The spine is a structural element the page is organised around, not a
  decoration inside one diagram.
- The terrain finishes the page, and the line arrives there.
- Three blocks that read as three movements.
- Code that looks like code, in the page's own palette, on both grounds.

**Non-Goals:**

- No new colour, font, asset, or dependency. No highlighter library.
- No change to the masthead, hero prose, feature list, or pipeline artwork.
- No change to the console's behaviour, modes, transcripts, or states.
- No redesign of the terrain artwork or route geometry itself — the route's
  paths stay pixel-identical; only the band's position and its alignment to the
  axis change.

## Decisions

### D1 — The terrain is the last section

`.terrain` (renamed from `.landscape`) moves out of `.hero__pipeline` and becomes
a top-level section after the last block, before the footer.

*Alternatives:* leaving it mid-page and repeating a mountain at the end —
rejected as two massifs for one metaphor; keeping it in the hero and letting the
page trail off — rejected, it is the page's strongest close.

### D2 — One axis token, `--fold`

`:root{ --fold: 65.31%; }` of the shell content box, derived in a comment as
`--col-prose + 40.7% × (1 − --col-prose)`. Everything that must sit on the spine
uses it, and the hero keeps `40.7%` of its own column because that is the same
position expressed in that container's coordinates.

*Alternatives:* hard-coding a pixel offset per section — rejected, it drifts at
every width; deriving with `calc()` mixing a percentage token and a number —
not expressible in CSS.

### D3 — Spine segments that abut, not one absolutely-positioned overlay

Each section that needs a spine is a grid whose first track is its content and
whose second is the spine, sized `minmax(0, 1fr)`. A section's spine therefore
spans exactly that section's height, and since sections are contiguous block
boxes the segments join with no gap.

*Alternatives:* (a) one page-level absolutely-positioned line — rejected: it
would have to be masked wherever it crosses a plate or a dark ground, and it
cannot know where a section's content begins; (b) replacing the SVG spine with
CSS — rejected: the mask that hides the line behind each plate's front edge is
the detail that makes the hero work, and reproducing it in CSS means
re-implementing the plate geometry.

The hero keeps `.sig`. The CSS spine begins where `.sig`'s box ends (the
pipeline's bottom) and continues to the terrain. Overlap between the two is
possible and safe; a gap is not.

### D4 — The spine's width matches the plate stroke at every width

`.sig` is `stroke-width: 4` in viewBox units, so its rendered width scales with
the plate column: `4 × --plate / 364`. The CSS spine uses the same expression
(`--spine-w: calc(var(--plate) * 4 / 364)`) rather than a fixed 4px, so the two
are the same line when the plate is at either end of its clamp.

### D5 — The terrain is anchored by its left edge, derived from the axis

The art box moves from `right: var(--art-right)` to
`left: calc(var(--fold) - 1124.3 / 2172 * var(--art-w))`, so the route's fork is
on the axis **by construction** instead of by a tuned offset that `CORRECTIONS.md`
had to defend at seven widths. The right edge is free to bleed; `html{overflow-x:clip}`
already handles that.

The spine's bottom is lifted by `max(0px, calc(var(--band) - var(--art-w) * 0.3546))`
so it reaches the art box's top edge — the route trunk's `y=0` — even where the
band's clamp diverges from the art's height.

### D6 — Three blocks, three grounds

1. **Protocol** — paper. `rule-head`, a ruled ledger whose right column carries
   the mono facts, and the console plate sitting on the axis.
2. **Scripts** — full-bleed `#111110`, the page's single inversion. The spine
   crosses it in `--accent` (6.06:1 there). Carries the
   `model(fn(input)) → output` material and the Lua specimens.
3. **Operations** — paper. A dense ledger and the `emb-top` capture as a dark
   plate.

The ground change is the variety; nothing else about the blocks is arbitrary —
each uses the poster's atoms.

*Alternatives:* three paper blocks with different column counts — rejected as
variety of rhythm only, which at this length reads as one block repeated; a
photographic ground — rejected, the page has exactly one photographic surface
and it is the massif.

### D7 — Code tokens: four classes, two ground sets, measured

Classes: `.t-cmd` (command/keyword), `.t-str` (string literal), `.t-num`
(numeric), `.t-dim` (comment/meta/punctuation), plus the plain token.

| Class | Paper | Ratio | Dark | Ratio |
|---|---|---|---|---|
| plain | `--fg` | 17.3:1 | `--bg` | 16.59:1 |
| `.t-cmd` | `--fg` w700 | 17.3:1 | `--bg` w700 | 16.59:1 |
| `.t-str` | `--accent-ink` | 4.66:1 | `--accent` | 6.06:1 |
| `.t-num` | `--fg` tabular | 17.3:1 | `--bg` tabular | 16.59:1 |
| `.t-dim` | `--muted` | 4.82:1 | `--rule` | 9.22:1 |

Emphasis is weight and colour-role, never a second hue: the palette has one
accent and this decision does not spend it twice.

### D8 — The reveal and route logic follows the terrain

`main.js` needs no new mechanism: reveals are position-driven, so the blocks
reveal as they enter. The route's scroll window is the band's own geometry and
still computes from `getBoundingClientRect()`, so it works in the new position;
what changes is that the band is now the last thing before the footer, so the
route finishes at the page bottom rather than mid-page. The terrain's spine
segment and the route must be checked together at the handover.

### D9 — Impeccable governs the work

This is a **recomposition of a whole surface inside an established world**
(`new-work.md` §3, second case): the visual system is fixed, the composition is
open. Concretely:

1. `impeccable context` was run once for this target earlier in the session.
2. The surface brief at `website/.impeccable/surfaces/website-index-html.md`
   gains a new FIRST VIEWPORT/SECTION ORDER and a STORY update, written
   **before** the first markup edit, per `new-work.md` §5.
3. `reference/craft-floor.md` read immediately before editing. Two of its rules
   bind here: no coloured `border-left`/`border-right` above 1px as a code
   treatment, and no kicker above a heading.
4. Detector run once after the build over the changed files.
5. `DESIGN.md` is touched only if the user approves a durable system change.
   The `--fold` axis is a page-composition value, not a system token, so it
   lives in the stylesheet's section comment and in `website/README.md`.

*Alternatives:* a surface-scope `concept-seed` round — not run. The user
specified the composition's frame (terrain last, one spine, varied blocks, code
highlighting) and the visual world is pinned by `DESIGN.md`; the open work is
the block design, which is resolved here rather than fanned out into a
tournament. This is the "precisely specified" case `new-work.md` says to shape
directly, and it is recorded rather than assumed.

## Risks / Trade-offs

- **The spine crosses a dark ground and could read as a mistake.** → In the
  dark block the line is the brightest thing on the ground (6.06:1); it is the
  intended moment, and the block's type is laid out to clear the lane.
- **Segments join imperfectly at a boundary.** → Mitigated by grid-track
  abutment (D3) and by measuring the join at each boundary at three widths.
- **Moving the terrain breaks the route's scroll draw or its axis.** → The route
  is measured against the band at runtime, so the draw follows; the axis is
  re-derived by construction (D5) and verified by measurement at 1086/1440/390.
- **A full-bleed dark band is a big move and can read as a different site.** →
  The band uses the plates' own `#111110` and the page's own tokens and type
  ladder; it is the page's material inverted, not new material. If it reads as
  foreign, the fix is the band's measure, not a new palette.
- **Code specimens inflate the page and compete with the prose.** → Specimens
  are budgeted: one shell specimen in the protocol block, two in the scripts
  block; everything else stays prose and mono facts.
- **The page grows again.** → Accepted; the ratio was released in the prior
  change and is not a constraint here either.
- **The route no longer finishes mid-page.** → The band is last, so the route's
  progress denominator (`rect.height * 0.86`) is re-checked at the page bottom
  to confirm the trunk completes before the footer rather than being cut off.

## Migration Plan

Additive and static; rollback is a revert of the four site files. Order:

1. Markup: move the terrain, split the capability region into three blocks, add
   the spine elements and the code token spans.
2. Styles: `--fold`, `--spine-w`, the `.spine` primitive, the three grounds, the
   dark type ladder, the token sets.
3. JS: verify the route/reveal behaviour in the new position; adjust only if
   measurement shows a defect.
4. Measure, then document.

## Open Questions

- Whether the dark block sits second or third — deferrable; the two paper
  blocks are symmetric in weight and either order preserves the sequence.
- Whether the console belongs in the protocol block or as its own ground —
  deferrable; it is a plate on either, and the seam does not change.
