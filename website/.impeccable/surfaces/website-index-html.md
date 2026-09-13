---
version: 1
slug: "website-index-html"
primary_target: "website/index.html"
related_targets: []
---

# Surface brief — `website/index.html`

## Scope and visitor mode

**Persuade.** The marketing surface for `emb`. This brief now covers the whole
page's composition: the hero spread, the capability blocks, and the terrain's
placement. The masthead, wordmark, claim, sub, actions, six-row feature list and
pipeline artwork are fixed and must not move; everything from the terrain down
is being recomposed.

## Audience, job, and action

Backend, platform, and application engineers who already operate Redis and must
believe, in under a minute, that `emb` drops into the stack they have. Their job:
see the interface they would actually type, understand what beyond embeddings it
does, and leave for the install command. Primary action is the existing hero CTA;
the blocks remove the reasons to hesitate.

## Proof and content

Every line is repository-sourced. The console replays real transcripts
(`EMB` → float32 bytes; `EMB … VALUES` → the envelope; `EMB.SCRIPT LOAD` →
`EMB.EVSHA` → a labelled classifier). The scripts block carries the repository's
own Lua, including `examples/scripts/sst2.lua`. The `emb-top` panel carries the
dashboard's real captured render. No invented number, model, receipt or reply.

## Chosen direction and memorable moment

**The page is a signal travelling from the wordmark to the massif.** The orange
line stops being decoration inside one diagram and becomes the page's structural
axis: one unbroken rule from `emb` to the ridge, with the sections arranged
around it. The memorable moment is the inversion — the page goes dark once, in
the middle, and the orange line is the brightest thing on the ground.

## Constraints

- Inherit the world. No concept tournament, no replacement identity, no
  `DESIGN.md` rewrite. The visual system is fixed; the composition is open.
- Atoms only: `rule-head` + rule, ruled ledgers, plates, the axis, the
  annotation form. No cards, gradients, radii, or decorative shadows.
- No new colour, font, asset, or dependency. The dark block is the page's own
  `#111110` / `#292823` material inverted, not new material.
- Type floor 12px desktop / 14px mobile holds on every ground, and every text and
  control colour is measured against its own background.
- Code is typeset as code with four token classes, on paper and on dark, all
  contrast-checked. Emphasis is weight, never a second hue.
- No coloured `border-left`/`border-right` above 1px as a code or callout
  treatment, and no kicker above a heading (craft floor).
- `prefers-reduced-motion` collapses playback. Every ground must read with
  JavaScript disabled.
- No hosted endpoint, no live service, no benchmark number without provenance.

## Unresolved decisions

- Whether the dark block sits second or third.
- Whether the console belongs in the protocol block or on its own ground.

Neither changes the spine, the grounds, or the tokens.

## Direction contract

THESIS: The page's one idea is that the protocol is the integration, and its one
line is the signal that carries it. It refuses the category default of stacked
full-width cards by making a single orange rule the structure the whole page is
organised around, and by letting the page invert once rather than varying
everything slightly.

OWN-WORLD: The palette is unchanged — `#F3F0E8` paper, `#0B0B0B` ink, `#FF5A1F`
as surface, `#C23D00` as ink-accent — and the component language is the poster's:
hairline rules, tracked mono labels, isometric plate faces (`#111110`,
`#292823`). With all content removed, what remains is a ruled ledger, a black
parallelogram, and one orange vertical line drawn through both.

STORY: The visitor understands that any Redis client already speaks this, that a
script can make the same server answer as a classifier, and that operating it
uses commands they already know. They leave for the install line with no second
service to evaluate — and the page closes on the massif rather than trailing off.

FIRST VIEWPORT: Unchanged — the poster's masthead, meta band, oversized `emb`,
and the claim/sub/actions/prose column beside the four-plate isometric stack.

SECTION ORDER: the hero spread; then three capability blocks each carrying a
segment of the same spine — paper (protocol ledger + console plate), a full-bleed
dark inversion (scripts and code), paper (operations ledger + `emb-top`); then the
terrain, last before the footer, where the spine becomes the ridge route and
finishes. The massif is the page's full stop.

The spine is one line at every boundary: the hero's masked SVG spine, the CSS
segments in the blocks, and the route are all on one axis (`--fold`, derived from
the hero's own geometry) and at one width (`4 × --plate / 364`). No gap, no step.

FORM: Whole-surface recomposition inside an established world (`new-work.md` §3,
second case). No ordered list and no seed key: the visual system is fixed by
`DESIGN.md` and the composition's frame was specified by the user, so the roll is
deliberately skipped and the open work — the block designs — is resolved in
`design.md` instead of fanned out.

FINISH: unreviewed and undocumented is unfinished; this build ends with the
finish review, the verdict, DESIGN.md, and every shipping raster carrying its
provenance.
