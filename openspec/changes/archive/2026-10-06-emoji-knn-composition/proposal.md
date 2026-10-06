# Proposal

## Why

The algebra is the only part of the zone a visitor can play with, and playing
with it is a dead end. Every two-emoji query answers with one of the two emoji
that were typed: `+` averages direction, so the operands tie and float32
rounding picks the winner. Measured, `🍕+🦅` scores 0.7007 for pizza and 0.7007
for eagle, every third entry below both; across 11 food+animal country pairs, a
third entry won 0 times. A visitor types two emoji, gets one of them back, and
stops.

The fun is one operator away. Asking what two terms have in common instead of
averaging them answers with a third entry, measured: `🐉*🥟` → 🐼 panda,
`🍣*🌸` → 🍡 dango, `🗽*🏈` → 🏟 stadium, `🐘*🛕` → 🕉 om, `🐻*🍁` → 🦊 fox. What is
missing is not only the operator but a place to play: the plate is a query box
with three fixed starters, it offers no way to build a query from what it just
showed, and it shows a score without showing what the score is made of, so a
joke and a stretch look alike.

## What Changes

- A `*` conjunction operator: a k-nearest-neighbour joint-neighbourhood search
  that ranks entries by the product of their similarity to every term and
  returns the best entry that is not itself a term. `🍕*🦅` asks what pizza and
  an eagle have in common rather than averaging them.
- The reply carries the working for a conjunction: each result's similarity to
  each of the query's terms, so a surface can show why its terms agree and a
  weak joint answer reads as weak.
- The plate becomes playable: a reader builds a query by adding a term from what
  the plate just showed — tap a glyph in the ranking to compose with it, and
  choose the operator — and the name, the query, and the commands follow.
- The plate's starters are the shipped measured examples rather than three
  queries written into the page, so a discovered or re-pinned joke becomes a
  control without a page edit.
- The gallery's examples are discovered from the model by a search that reports
  the floors it used, re-measured against the zone, and gated against drift.
- The vocabulary gains the 262 flag entries CLDR puts beside its emoji
  annotations (1961 → 2223), so a country answer exists to be reached at all.
  Measured to be necessary but **not sufficient**: the food+animal flag joke is
  unreachable, and ships as a measured ceiling rather than as a promise.
- **BREAKING** (refusals): a name made only of operators is refused, so a lone
  `*` label is a termless name rather than a literal asterisk.

## Capabilities

### New Capabilities

- `emoji-joke-gallery`: how the zone's shipped examples are discovered from the
  model, measured, pinned with their scores, gated against drift, and offered to
  a surface — including the rule that a joke the model cannot tell ships as a
  measured ceiling instead of being omitted.

### Modified Capabilities

- `emoji-operations`: the conjunction composition and its joint-neighbourhood
  ranking, the working it carries, the refusal of mixed operators, and the flag
  entries joining the vocabulary.
- `embedding-demos`: the zone plate becomes playable — composed from its own
  results, driven by the shipped examples, and showing why a conjunction's
  terms agree.

## Impact

- `internal/emoji/grammar.go`, `internal/emoji/vocab.go`: the operator and the
  joint ranking.
- `cmd/emb-dns`: the conjunction path, and the working in the JSON document.
- `dns/tools/build-emoji-vocab.py` (reads `annotationsDerived/en.xml` too),
  `dns/emoji-vocab.json` (1961 → 2223), `dns/examples.json`, `dns/tools/`.
- `website/demos/dns.html` and a `website/tools/` build step that supplies its
  starters from `dns/examples.json`.
- `justfile` (`emoji-jokes`), `docs/dns.md`.
