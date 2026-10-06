---
version: 1
slug: "website-demos-dns-html"
primary_target: "website/demos/dns.html"
related_targets: ["website/demos/index.html"]
---

# Surface brief — the zone

## Scope and visitor mode

**Read and play.** One plate at `website/demos/dns.html` for `emb`, the thirteenth
and the gallery's first served by a second deployment. The visitor arrives from the
gallery or a search; the plate answers *what the model does when the client is
`dig`* — a DNS query is a sentence, the reply is one `TXT` record, and the whole
ranking is readable if you know where to look. The plate is an extension of the
gallery's committed world, not a new one: the palette, type ladder, five teaching
sections, dark-plate apparatus and caption discipline come from `DESIGN.md`
unchanged.

## The visitor's job

Type a sentence, a glyph, a composition, or a conjunction — or tap an emoji in
the ranking to compose with it — and watch it become a DNS name and come back as
an emoji sentence and a ranked list, the same answer a `TXT` record carries and
the same answer `dig` escapes into `\240\159\...`. Leave knowing that the record
holds one glyph and the working rides the HTTP surface (a conjunction's legs with
its joint score), that a sum of two emoji answers with one of them while a
conjunction can answer with a third, and that the zone is a separate machine
holding its own model.

## Constraints (inherited, non-negotiable)

- Inherit the world in `DESIGN.md`. No new colour, font, asset, dependency or
  component language. The plate spends its one accent on the top result's mark.
- Read with JavaScript disabled: the five sections and the exact commands are
  complete and static.
- No invented number, glyph or reply. The ranked list, the escaped pairing, the
  model, the dimension and the entry count all come from the service's own reply
  and metadata route; the examples carry the measured results in
  `dns/examples.json`.
- The plate names the surface that answered it and reports *that* surface's model
  and vocabulary size: it is the one plate the gallery serves from another
  deployment, and it says so.

## Direction contract

THESIS: The answer is one record, and the record is a sentence someone typed. The
plate is a **record card**: the query drawn as its own labels on the atlas
graticule, the ranked answer as ruled marks, and the same answer printed twice —
the escaped `TXT` data a resolver hands back beside the glyph it stands for. It
refuses the category default of an input box beside a list: the wire form and the
readable form are the point.

OWN-WORLD: The gallery's own material — ruled captions, tracked mono labels, the
dark plate, the isometric graticule, `--accent` on the top mark. The emoji are
the only chromatic material, and they are the subject's colour: never tinted,
framed or shadowed.

STORY: The visitor understands that a DNS name is a query and its `TXT` record is
the answer; believes the zone is the same retrieval as the rest of the gallery;
and, with the service down, watches the plate keep its explanation and examples
and state which surface failed rather than invent an answer.

FIRST VIEWPORT: Plate number and title in the display face (`V. The zone`),
the plate's own `WHAT YOU ARE LOOKING AT` paragraph, and the caption line read
from the service — model, dimensions, vocabulary entries, TTL — in the existing
`FIG.` voice. Nothing above it but the masthead and the top link to the next
plate.

FORM: The record card, an extension of the committed gallery world rather than a
new one; the dealt seed key is `cb76e03d` and the brief's pinned direction (design
D10) overrides it, so the roll settles composition, not the world.

FINISH: unreviewed and undocumented is unfinished; this build ends with the
finish review, the verdict, DESIGN.md, and every shipping raster carrying its
provenance.

---
## Detector

`impeccable detect --json` over `demos/dns.html`, `demos/index.html`,
`demos/laya.html` and `assets/css/styles.css`, run once. `dns.html` carries 13
findings against the gallery's standing 12 (`demos/similarity.html`): wide
tracking, cramped padding on the ruled blocks, all-caps mono labels, the Inter
family, one em-dash-overuse, and the html/body positioned-child clip the skip
link already raises on every plate. The one finding beyond the standing set is
the `rig__pick` control's own cramped edge — the shipped component wherever it
is used, not this page's markup. `styles.css`'s single finding (Inter, line 20)
predates the `.rec` block. Nothing in this page's own markup was flagged, so
there was no mechanical defect to invent a token for; the standing findings are
recorded, not papered over.

## Finish review

Verdict: ships. Against the direction contract the plate is the record card it
promised — the query name drawn as its own labels, the ranked answer as ruled
marks with the top rule the one accent, and the same answer printed twice, the
escaped `TXT` data a resolver hands back beside the glyph it stands for. The
first viewport carries the number and title, the `WHAT YOU ARE LOOKING AT`
claim, and the caption read from the service's `/meta` (`emojiml · 384
dimensions · 2 223 emoji · 300 s TTL`), so the plate reports the surface that
answered it and no figure is typed. Its example controls are the zone's shipped
set, stamped from `dns/examples.json` by `website/tools/stamp-examples.py`.

Three material defects were found in the live render and fixed in one batch: the
`answered` state showed the literal word because the empty label fell through
`||` (now `??`); the example controls never showed which was live (`aria-pressed`
now follows the choice and is cleared by a typed name); and a grammar refusal
offered a useless "retry" and did not name the resolver's answer (it now says a
resolver answers `NXDOMAIN`, and only the unreachable state offers a retry).

The second round confirms them: the desktop and mobile captures show the pressed
example, the empty answered state, and the record card drawn from one reply, and
reduced motion draws all four mechanism stages and the ranked rows in one frame.
The confirmation is the same run that produced `dns-desktop.png` and
`dns-mobile.png`; no third round was spent.
