---
version: 1
slug: "website-demos-index-html"
primary_target: "website/demos/index.html"
related_targets:
  - "website/demos/vector.html"
  - "website/demos/similarity.html"
  - "website/demos/search.html"
  - "website/demos/atlas.html"
  - "website/demos/lens.html"
  - "website/demos/function.html"
  - "website/demos/image.html"
  - "website/demos/batch.html"
  - "website/demos/cache.html"
  - "website/demos/graph.html"
---

# Surface brief — the demos gallery

## Scope and visitor mode

**Read.** Ten plates at `website/demos/` for `emb`. The visitor already
operates Redis and has decided the product is worth a look; this surface answers
*what an embedding is and why the server is worth having* by running the real
sandbox and a real shipped index, not by describing them. The gallery is the
long-form companion to the landing and the docs: explanation first, reference
second.

## The visitor's job

Arrive with no mental model of vectors, leave with one — and with the exact
commands each idea is made of. Each plate stands alone; the index gives the
reading order. The proof is always the same: a real command, a real reply, a real
figure, and a page that says so when the sandbox is unreachable rather than
fabricating one.

## Constraints (inherited, non-negotiable)

- Inherit the world in `DESIGN.md`. No new colour, font, asset, dependency, or
  component language. Atoms only: ruled captions, tracked mono labels, the
  isometric plate, the terrain break.
- Every plate reads with JavaScript disabled; the interactive rig is an
  enhancement over a complete static explanation.
- No invented number, model, receipt or reply. Every figure is the server's or
  the manifest's own.
- Type floor 12px desktop / 14px mobile. Emphasis is weight, never a second hue.

## The five teaching sections

Every plate keeps its fixed order — `WHAT YOU ARE LOOKING AT` → `TRY IT` →
`WHAT JUST HAPPENED` → `WHY IT MATTERS` → `THE EXACT COMMANDS` — tightened
rather than merged (change `website-demos-tldr`, 2026-09). Prose is one idea per
sentence and no paragraph carries two arguments; the mechanism that a long
enumeration used to describe is the reader's own instrument.

## Amendment — the read-mode switch (change `website-demos-tldr`)

**Scope: a control at the top of the content, and the gist it reveals. Not the
masthead.** The masthead is shared with the landing and is out of scope; the
switch lives inside `<main>`, in a ruled bar above the plate, on `/demos`,
`/docs` and `/gem`.

**What it does.** One button (`[data-tldr-toggle]`, `aria-pressed`) sets
`data-tldr` on `<html>`, remembered in `localStorage` under `emb.tldr` and
applied by `assets/js/tldr.js`. On the gallery and the client page it keeps the
plate, each page's one-paragraph `.tldr` gist, `TRY IT` and
`THE EXACT COMMANDS`, and hides the three argument sections and the terrain
breaks. On `/docs` it hides `.doc-prose` and keeps the structure, the command
lists and the code.

**Direction contract**

THESIS: The gist is what a reader acts on — the plate's claim in plain words, the
instrument, and the command — with the argument it is made of folded away. It
refuses the category default of a second, dumbed-down page: the two readings are
the same document, and the switch is a view, not a copy.

OWN-WORLD: The control is the page's own material — a hairline-ruled bar, a mono
label, a square bordered button that inverts on press. Nothing is added to the
palette or the component language.

STORY: A reader who wants the shape of the gallery gets ten one-line claims and
ten live plates; a reader who wants the argument opens the full plate.

**How the constraints are met**

- *Reads with JavaScript off.* The button carries `hidden` and is revealed only
  by the script, so scripting-off shows the full page and no dead control.
- *Contrast.* `--muted` on paper is 4.82:1, the button is `--fg` on `--bg` and
  inverts to `--bg` on `--fg`; hover is the accent with `--fg` ink, as the
  buttons already do.
- *No new claims.* A `.tldr` line states only what its plate already states; the
  switch adds no number, model or receipt.

**Detector.** `impeccable detect --json` over the gallery, the client page, the
docs page and `styles.css` was run in place and compared, finding by finding,
against the same run over a `HEAD` worktree: 93 findings each, the same set, and
zero naming the switch. The site's standing findings (cramped `clamp()`
padding, tracked mono labels) are unchanged and were already recorded.

---
## Amendment — the mechanism, in stages (change `website-demos-tldr`, 2026-09)

Every plate opens `TRY IT` with the same figure: the steps between the
reader's input and the answer, as a run of glyphs joined by the page's rule.
Before a run every step is dim, so the strip is a map of what is about to
happen; on a run the steps light in order beside the real call, and a step's
accent mark (the one vector that matters, the row a search lands on) is the
only thing that changes colour. It is the gallery's one animation that
explains rather than measures, and it is the answer to *show the mechanism,
do not narrate it*.

**One renderer, ten specs.** `mechanism()` and `playStages()` live in
`assets/js/demos.js`; a plate names its steps and nothing else, so ten plates
cannot invent ten visual vocabularies. The glyph vocabulary is fixed and
small — text, tokens, model, vector, search, rank, score, labels, passes,
cache, photo, fanout, graph — drawn in the same 56×44 box at the same two
weights.

**The motion contract holds.** `playStages()` reads the same
`prefers-reduced-motion` as every other figure: with reduced motion the strip
is drawn complete in one frame and nothing depends on the animation having
run; the plate calls `all()` when the real answer lands, so a slow sandbox
never leaves the strip mid-stride.

**No figure is labelled.** The `FIG. n —` prefixes were removed from every
plate, the gallery index and the client page in the same change: a plate's
ruled caption now opens on its own name, and the mechanism's own steps are
the only numbering the reader needs.

**Values.** Every demo was run against a local `emb` + bridge and its
readout read back: vector (384 dims, norm 1.0000, 1 536 bytes), similarity
(0.754 for a paraphrase), search (7 of 2 782, reranked), atlas (2 782 marks,
10 regions), lens (two spaces, shared-neighbour count stated), function (four
reply shapes), image (CLIP ViT-B/32, 8 labels), batch (one call vs six),
cache (miss then hit), graph (8 nodes, 16 edges). One wrong static fallback
was corrected — the index said 6.4 MB where the shipped manifest is 6.1 MB —
and long floats in the readouts are shown at four places.

**Detector.** `impeccable detect --json` over every changed HTML file and
`styles.css`, compared finding by finding against a `HEAD` worktree: 204
findings now against 208 there, the difference four resolved (em-dash and
all-caps), and nothing naming the mechanism.

---
## Amendment — the plate's own step, and a stale module (change `website-demos-tldr`, 2026-09)

**Advance to the next demo.** Every plate now closes on a ruled row naming the
plates either side of it — `Previous` / `Next` and the plate's own name in the
display face — so a reader who arrived from a search or a link can keep going
without returning to the gallery. The first plate carries only `Next`, the last
only `Previous`; the row is always visible, including in the gist, because it is
navigation and not an argument.

**A stale `demos.js` broke every plate.** `demos.js` keeps a stable name while
its exports change; the local dev server sent `Cache-Control: no-store` for the
HTML only, so a browser kept a heuristically cached module from before the
mechanism landed and every plate died on `does not provide an export named
'mechanism'` — a hard failure, because an ESM named import is static. Fixed at
the source: `website/tools/dev-server.py` now answers every request `no-store`
and names `demos.js` / `tldr.js` from their own mtimes in the served HTML, so
the working loop can never outlive an edit. The published tree is unaffected:
`_headers` already pins `max-age=0, must-revalidate` for `/assets/js/*`, which is
the right policy for a stable name whose bytes move.

---
## Amendment — eleven plates, a switch that resets, and a shared strip (2026-09)

**The languages (plate 6, `website/demos/multilingual.html`).** The sandbox
loads a multilingual model, so the gallery now shows what it is for: one
English sentence and its translations in French, Spanish, German, Italian and
Portuguese, each compared with the original, beside two unrelated sentences.
Measured on the local sandbox: translations 0.943–0.964, unrelated −0.055–0.060,
a gap of 0.883 across six languages and one 384-dimension space. The calls are
sequential, not parallel: the bridge caps commands in flight, and seven at once
are refused `at capacity`.

**The reading order is now eleven**, and the gallery index says so in the
teaser, the gist and the ruled caption; the nav chain runs lens → languages →
function.

**The reading switch resets on navigation.** `tldr.js` no longer reads or writes
`localStorage`: the mode is a choice about the page in front of you, and it
starts off on every load. Switching now carries a gesture — the gist line rises
in (`tldr-rise`) and the page cross-fades (`tldr-settle`), cleared by a
`tldr-switching` flag — both standing down under `prefers-reduced-motion`.

**The mechanism moved to `assets/js/mechanism.js`** so the client page and the
documentation draw it without loading the gallery's sandbox client. `demos.js`
re-exports it, so the plates' imports are unchanged; the module also adds the
`wire`, `lanes`, `pool`, `reply` and `script` glyphs the other surfaces need.
