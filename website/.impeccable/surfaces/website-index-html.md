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
## Amendment — the `emb-top` plate plays the dashboard's own recording (change `website-emb-top-recording`)

**Scope: the plate in the operations block, and nothing else.** The ledger, the
facts list and the section's other captions do not move. Where the brief above
said the `emb-top` figures live on the docs surface, this amendment supersedes
it: the landing keeps a plate, and the plate is a capture — of the dashboard's
recording, replayed in the dashboard's own text.

**What changes.** `.topviz__screen` — a `<pre>` of spans whose own caption read
"Illustrative render · not a measured run" — keeps its element and its measure.
It carries a real frame of a real run as its still state, written by
`just website-topviz`, and that frame is replaced by the player replaying the
same take's cast wherever the reader's motion preference allows it. The caption
becomes a receipt: `Measured run · <date> · emb-top v<version> · <n> models ·
<addr>`. The documentation keeps the animated capture it already carries.

**Direction contract for the plate**

THESIS: The plate is evidence, and evidence here is a terminal. It is one real
run of one real node, replayed from the dashboard's own recording at the plate's
own type size, captioned with when and what. It refuses both category defaults —
a drawing of a dashboard nobody ran, and a downscaled screenshot of one — and it
refuses the third one the previous revision settled for: a still frame standing
in for a thing whose whole point is that it moves. Motion and legibility were
treated as a trade; the recording's own text makes them the same choice.

OWN-WORLD: One new kind of thing enters the world, and it is admitted on
purpose: a vendored player, drawn in the plate's colours, sized to the plate's
grid, and served from this origin. Nothing else is added — no third-party
request, no raster, no second animation policy. The plate is still made of the
page's own mono at the page's own measure.

STORY: A reader who has just read the operations ledger watches the same claims
the dashboard renders — per-model rates, an activity heatmap, two stream charts,
cache, CPU and memory — move under load, and can stop them and read the numbers.

FINISH: The plate animates, so it is gated the way the rest of the page is gated:
`prefers-reduced-motion` decides whether it plays, a control stops it, and with
scripting disabled it is a complete still frame rather than an empty plate. The
run's figures are also written into the page as a sentence for the narrow layout
and for screen readers.

**Constraints carried over, and how each is met**

- *Type floor (12px desktop / 14px mobile) on every ground.* The plate keeps
  `clamp(12px, 1.02vw, 13px)`. It is a terminal replay, so its cells are the
  page's type at that size, sharp at any zoom.
- *Every ground reads with JavaScript disabled.* The frame is markup and paints
  first; the player replaces it only where it can run.
- *No invented number, model, receipt or reply.* Every character the plate shows
  is the recording's own output, and the caption, the frame and the played take
  are written by the capture tool, not by hand.
- *Every version string is generated.* The plate's version is frozen in the take
  and written into the caption by the same tool, so it is stated as the version
  the run *showed* rather than as the current release. The player's own version
  lives in its filename, not in the page's copy.
- *Inherit the world.* One deliberate departure, recorded here: the plate's
  `line-height` is one cell, not the page's leading. A terminal's frame borders
  and chart axes are drawn with box characters that only meet when a line box is
  one cell tall; at the page's 1.7 the verticals arrive as dashes and each
  model's meta line drifts from its row. The player is themed to keep that grid
  rather than to fight it.
- *Motion is a choice, not a default.* The plate autoplays only under
  `prefers-reduced-motion: no-preference` and always offers a pause.

**What the page pays, and what it gains.** The plate is no longer free: 181 KB of
player script, 19 KB of player stylesheet and ~850 KiB of asciicast, all from
this origin, against a still frame's nothing. The take is the one number worth
knowing — it is escape sequences and per-cell colour, so it gzips to ~16 KiB, and
`_headers` declares its type so that is what a reader downloads; the scripts are
`defer`red and the plate is not created at all below the narrow breakpoint, where
there is nothing on screen for it to animate. What the plate gains is the motion
and the heatmap's colour the previous revision sent away to the docs, at the
plate's own measure — and it still costs the page no image of the dashboard.

**Detector.** `impeccable detect --json website/index.html
website/assets/css/styles.css` was run once after the build, in place, and its
output compared, finding by finding, against the same run in a worktree of
`HEAD`: 18 findings there, 20 here, and every one of the two is accounted for.
`tight-leading` (1.05) is the frame's one-cell leading — the deliberate departure
recorded above, which the still-frame revision introduced and this one keeps,
and now hands to the player as an option so both states share the grid.
`cramped-padding` names `.topviz__play`, the plate's new wrapper: the finding is
the page's existing `clamp()`-padding false positive (it is reported for
`.console`, `.block`, `.uses`, `.shift` and `.hero__start` at `HEAD` too), the
measured inset is 18px at 1280, and writing that padding as a literal makes the
finding disappear — checked by running the detector over a copy with
`padding: 18px`. The responsive step is worth more than the silence, so it is
accepted and recorded rather than flattened away. An earlier run over the
image-plate revision reported two findings of its own, both fixed: the caption
sat inside the dark plate, putting `--muted` on `#111110` at 3.4:1, and the
mobile figures used `--rule` (a hairline token) as body text at 1.8:1 on paper.
The caption is a paragraph beside the plate — where it was before the change —
and the figures use `--muted`.
