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
