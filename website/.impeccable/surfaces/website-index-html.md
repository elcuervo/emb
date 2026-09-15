---
version: 1
slug: "website-index-html"
primary_target: "website/index.html"
related_targets: []
---

# Surface brief — `website/index.html`

## Scope and visitor mode

**Persuade.** The marketing surface for `emb`. This brief covers the whole page's
composition: the hero spread, the capability blocks, and the terrain's
placement. The masthead, wordmark, claim, sub, actions, six-row feature list and
pipeline artwork are fixed and must not move; everything from the terrain down
is being recomposed.

**This surface is now the poster only.** The reference material moved to
`website/docs/index.html` (see `website-docs-index-html.md`). The landing keeps
exactly four things beyond its composition, each earning its place as a
pre-click decision: one copy-paste install line, the one REPL proof
(`redis-cli EMB minilm "hello world"` and its bytes), the pre-1.0 status with
the licence and platform list, and one measured `BENCHMARK.md` figure with its
reproduction command. Everything else — command tables, the YAML block,
reply-format prose beyond the one line that carries the argument, the `emb-top`
figures, and every version string — lives on the docs surface only.

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
- The landing carries no command table, no configuration block, and no more than
  four named commands per `facts` list. Its `Docs` link points at `/docs`, and it
  carries exactly one measured figure and exactly one install line.
- No claim about an input path the shipped scripts do not implement. `images` is
  removed from the hero sub and the INPUT note: `examples/scripts/siglip2.lua`
  documents that its image branch is absent and `pixel_values` is fed as zeros.
- Every version string is generated from `VERSION`, never typed. The page states
  `0.4.0.pre4` and that interfaces may still move.

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

The masthead's `Docs` control now points at `/docs` rather than a GitHub anchor.

---

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

---

## Amendment — the `emb-top` plate is a capture (change `website-emb-top-recording`)

**Scope: the plate in the operations block, and nothing else.** The ledger, the
facts list and the section's other captions do not move. Where this section
above said the `emb-top` figures live on the docs surface, this amendment
supersedes it: the landing keeps a plate, but it is a measured capture rather
than a hand-built render.

**What changes.** `.topviz__screen` — a `<pre>` of spans whose own caption read
"Illustrative render · not a measured run" — becomes a captured recording of the
dashboard under load, taken by `just website-topviz`. The caption becomes a
receipt: `Measured run · <date> · emb-top v<version> · <n> models · <addr>`.

**Direction contract for the plate**

THESIS: The plate is evidence, not decoration. It is one real run of one real
node, recorded against a scripted load and captioned with when and what, and it
refuses the category default of UI b-roll by being rendered in the page's own
material rather than a vendor's dark theme — the same plate colour, the same
paper ink, and the page's accent carrying the request-rate stream.

OWN-WORLD: Nothing new enters the world. The capture's palette is `#111110`,
`#F3F0E8` and `#FF5A1F` (mapped onto the dashboard's own ANSI slots), so the
"no new asset, colour, font or dependency" constraint is met by an asset *made
of* the page's material. No script, no player, no second runtime.

STORY: A reader who has just read the operations ledger sees the same claims
moving — per-model rates, a latency heatmap, cache, CPU and memory — and can
read the numbers off the frame.

FINISH: `prefers-reduced-motion` serves the still frame of the same run, so the
page's motion rule now covers the one element that would otherwise animate
without consent. Because the gate is in `<picture>`, this also holds with
JavaScript disabled, which the old `<pre>` did not need to prove.

**Constraints carried over, and how each is met**

- *Type floor (12px desktop / 14px mobile) on every ground.* The capture is 16px
  type in a 1175px frame, so the plate scrolls horizontally below the width that
  would take it under 12px rather than shrinking the type with it — the same
  treatment the `<pre>` had, and the reason the plate carries no visible
  scrollbar.
- *Every ground reads with JavaScript disabled.* The still frame is the `<img>`;
  the animation is a `<source media>` that simply does not match.
- *No invented number, model, receipt or reply.* Every figure in the frame comes
  from the run; the dashboard's figures are additionally written into the page as
  text by the capture tool, so the plate's numbers are not image-only.
- *Every version string is generated.* The plate's version is frozen in the
  recording and written into the caption by the same tool, so it is stated as
  the version the run *showed* rather than as the current release.

**Detector.** `impeccable detect --json website/index.html
website/assets/css/styles.css` was run once after the build, in place, and its
output compared against the same run over the files at `HEAD`: 16 findings
before, the same 16 after, none introduced by this change. The run over the new
markup first reported two of its own, both now fixed: the caption sat inside the
dark plate, putting `--muted` on `#111110` at 3.4:1, and the mobile figures used
`--rule` (a hairline token) as body text at 1.8:1 on paper. The caption is a
paragraph beside the plate again — where it was before the change — and the
figures use `--muted`.
