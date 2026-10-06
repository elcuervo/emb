# Design

## Context

See proposal.md — Why. The pieces and their bounds, all measured against the
running zone (`just dns-dev` + the numpy/redis-cli probes recorded below):

- `internal/emoji.Index` already holds the vocabulary as a normalized
  row-major matrix, and `Rank` costs one dot product per entry. Everything a
  joint ranking needs is already resident.
- `+`/`-` are composed in `scripts/emoji.lua`, one scripted call per query, and
  the script system's limits are the reason the algebra lives there and not in
  Go: a fresh `LState` per evaluation, a 64 KB script cap, and no matmul.
- The arithmetic reading is pinned by `dns/examples.json`, above all
  `🦈-🐟+🐦` → 🐦 0.731 / 🦆 0.689 / 🐔 0.654. `website/demos/dns.html` also
  carries a listing of `scripts/emoji.lua` inline.
- The current operator is a bisector, provably. For unit `A`, `B`:
  `cos(A+B, A) == cos(A+B, B) == sqrt((1+cos(A,B))/2)`. Measured over 16 pairs
  the two legs agreed to ~1e-8, `top1` was always one of the two operands, and
  the winner came down to float32 rounding (`🍕+🦅`: 0.7007/0.7007; `🐛+⏰`:
  0.71654002350 vs 0.71653996631).
- CLDR's `common/annotations/en.xml` (what the builder reads today) contains
  zero flags; the 262 flags are in `common/annotationsDerived/en.xml`, where
  each flag's only keyword is `flag`, giving the description
  `flag: United States`.
- `website/demos/dns.html` already asks the zone's read route live, renders each
  result's name, score bar, and escaped bytes, and unfolds the `dig`/`curl`
  commands. Its three starters are written into the page, while
  `dns/examples.json` is the verified set `just verify-emoji` checks against the
  zone — the two agree only by hand, and only because someone copied them.

## Goals / Non-Goals

**Goals:**
- A composition that can answer with a third entry, by searching for what its
  terms jointly point at instead of averaging them.
- Every existing arithmetic answer stays byte-identical.
- The gallery's examples are found from the model and gated against drift, and
  a joke the model cannot tell ships as a measured ceiling rather than as copy.

**Non-Goals:**
- Making `🍕*🦅` answer 🇺🇸. Measured unreachable (§ The ceiling). Doing so needs
  flag entries that carry associations, which is a data decision, not a ranking
  one.
- A relation operator (`king - man + woman`). There is no head term to subtract
  from in a conjunction, and `+`/`-` already cover arithmetic.
- Changing `scripts/emoji.lua` or the variance of `+`/`-`.
- A vocabulary-browsing route or an emoji picker palette. Tap-to-compose (§ D7)
  removes the need; listing the vocabulary is a different surface with its own
  question (what to do with 2223 glyphs).

## Decisions

### D1. `*` is a new operator; `+`/`-` are untouched

Terms joined by `*` are a conjunction. This keeps the pinned arithmetic
examples byte-identical, keeps `scripts/emoji.lua` (and the plate's inline
listing of it) valid without a re-stamp, and gives each operator one meaning.

*Alternative rejected:* make joint ranking the default for additive queries and
re-pin the arithmetic examples. It silently rewrites five pinned answers and the
plate's copy, and it throws away the reading of `a-b+c` as arithmetic, which is
a real question worth being able to ask.

### D2. A conjunction ranks by the product of its terms' cosines, operands excluded

`score(c) = ∏ cos(c, t)` over the query's terms, with the terms themselves
excluded, ties keeping vocabulary order. Three policies were measured over 11
food+animal country pairs and the whole vocabulary:

| policy | `🍕*🦅` winner | third entry wins | why |
|---|---|---|---|
| bisector (today) | 🍕 (or 🦅, by rounding) | never — operands tie at `sqrt((1+cos)/2)` | an average cannot prefer a stranger |
| RRF over each term's top-50 | 🍽 fork_and_knife_with_plate | yes | rewards a neighbour of *one* term; a term's own cluster dominates |
| product of cosines | 🐧 penguin (legs 0.20/0.65) | yes | a candidate matching only one term is penalized, which is what "both at once" means |

The product is `3CosMul` without its head term — there is no head term in a
conjunction, so the subtract direction does not apply. Excluding the operands is
what forces an answer that is not merely the query restated.

### D3. The joint ranking runs in Go, over the zone's index

The zone already holds every term vector (the `Rank` matrix) and already has an
`Embedder.Embed` for the spelled terms, so a joint rank is one extra pass with
one multiply per term — work the Lua surface cannot do at all (§ Context). The
`EMB.EVSHA`/preset path stays the `+`/`-` implementation.

*Alternative rejected:* a joint rank inside the script (`emb.math.mul` plus a
matmul). Same state and size ceilings, and it would push the vocabulary matrix
into Lua.

### D4. The vocabulary reads both annotation files

`annotationsDerived/en.xml` is fetched at the same pinned ref and merged, then
sorted by slug as today; a duplicate slug is refused by the existing loader
rather than folded silently. Measured effect: 1961 → 2223 entries (+262 flags),
so the boot index grows ~13% and the reported entry count changes. The asset
records both source URLs with a digest each.

Adding entries moves every pinned score, so `dns/examples.json`, the gallery
copy, and the model/entry metadata must be re-measured in the same commit. The
drift gate turns that into a reviewable diff.

*Alternative rejected:* leave flags out. A country joke would then have no
country to answer with at all. It is a necessary part of the genre — but see D6,
it is not sufficient, and nothing here should read as a promise that it is.

### D5. Discovery excludes near-duplicate pairs

With no floor, "the best joint third entry" is dominated by pictographic
siblings: harpoon barbs at legs 0.97–0.99, quotation marks, quadrant circular
arcs, family emoji. Those pairs resolve to the sibling of the same symbol, not
to a joke. The search therefore considers pairs whose terms are dissimilar
(cosine below a ceiling), requires both legs of the winner to clear a floor, and
prints both numbers so a re-run is comparable. Measured floors, and the ones the
tool ships: ceiling 0.35, floor 0.28, and a shared word counting as siblinghood
only when it appears in fewer than 30 entries — two clock faces share `clock`
(29 entries) and are siblings, while a dragon and a panda share `animal` (122)
and are not.

Two shapes the search had to take, both found by measuring:

- **Pairs cannot come from a neighbour scan.** Two unrelated terms are never
each other's neighbours, which is exactly what makes their answer a joke: a
scan of the nearest neighbours reported 11,926 triples and missed every country
pair. Candidate pairs come from the entries' own neighbourhoods, and each pair
is then scored against the *whole* vocabulary, because a winner can have one
weak leg and still win — a temple and an elephant answer with `om` at 0.261 and
0.634, beating every entry that is close to both. That pass runs in 7 seconds
over 430,168 pairs.

What that leaves is the genre that works, measured on the country pairs:
`🐉*🥟` → 🐼 panda (0.30/0.34), `🍣*🌸` → 🍡 dango (0.44/0.34), `🗽*🏈` → 🏟 stadium
(0.34/0.41), `🐘*🛕` → 🕉 om (0.26/0.63), `🐻*🍁` → 🦊 fox (0.49/0.17).

### D6. The ceiling: the flag joke needs data, not ranking (non-goal)

Every country pair, under every policy, was measured against the flag it wants.
The flag never wins, and the kNN winner is usually a far better joint entry:

| pair | wanted | wanted's legs | joint winner | winner's legs |
|---|---|---|---|---|
| `🍕*🦅` | 🇺🇸 | 0.11 / 0.16 | 🐧 penguin | 0.20 / 0.65 |
| `🍔*🦅` | 🇺🇸 | 0.11 / 0.16 | 🐔 chicken | 0.20 / 0.85 |
| `🦘*🐨` | 🇦🇺 | 0.32 / 0.35 | 🐵 monkey_face | 0.46 / 0.47 |
| `🐘*🛕` | 🇮🇳 | 0.18 / 0.36 | 🕉 om | 0.26 / 0.63 |
| `🍝*🍕` | 🇮🇹 | 0.32 / 0.22 | 🍽 fork_and_knife_with_plate | 0.44 / 0.52 |
| `🍣*🌸` | 🇯🇵 | 0.33 / 0.14 | 🍡 dango | 0.44 / 0.34 |
| `🐻*🍁` | 🇨🇦 | 0.13 / 0.21 | 🦊 fox | 0.49 / 0.17 |
| `🗽*🏈` | 🇺🇸 | 0.32 / 0.25 | 🏟 stadium | 0.34 / 0.41 |
| `🌮*🌵` | 🇲🇽 | 0.59 / 0.14 | 🏜 desert | 0.26 / 0.49 |
| `🐉*🥟` | 🇨🇳 | 0.10 / 0.21 | 🐼 panda | 0.30 / 0.34 |
| `🩰*🥐` | 🇫🇷 | 0.10 / 0.37 | 🥿 flat_shoe | 0.68 / 0.16 |

The cause is the data, not the math: `flag: United States` has no more in common
with pizza or eagle than any other country name does, and the closest pair of
legs (🦘*🐨 → 🇦🇺 at 0.32/0.35) is still beaten by a monkey. The genre is
reachable — the winners above are jokes — the *country* version is not. So
`🍕*🦅` ships as the ceiling example: the answer is 🐧 penguin, and the copy says
the flag is unreachable and why. Making it reachable means giving flag entries
descriptions that carry their associations, which is a separate, deliberate
decision about vocabulary data.

### D7. The interaction is tap-to-compose, with the working shown

The plate already answers live and already prints the escaped bytes and the
commands, so the play surface is one affordance away from the ranking it is
showing:

- **A ranked result is addable as a term.** The emoji a reader wants next is in
the answer they just got, so tapping a glyph appends it to the query (with the
operator toggle deciding whether it joins by `*` or `+`/`-`) instead of asking
them to find, type, or paste a glyph — typing emoji is the real barrier on a
phone, and a picker would need a vocabulary-listing route that does not exist.
- **`*` is what a tap defaults to.** A reader playing with two emoji is asking
what they have in common; `+`/`-` stay one toggle away for the arithmetic the
pinned examples demonstrate.
- **The working is shown for a conjunction.** Legs (each result's similarity to
each term) ride the HTTP document only for conjunctions: a single composed
vector has no per-term legs to report, so the arithmetic document is unchanged
and the existing surface contract holds. With the legs visible, the plate can
say why a joke is a joke — and say when an answer is a stretch.
- **Starters come from `dns/examples.json` at build time.** One `website/tools/`
step inlines the pinned examples into the plate (grouped by family) instead of
the page restating three of them. *Alternatives rejected:* hand-maintained
buttons (the drift this change removes — the page and the verified set agree
only by copying), and fetching the book from the zone at runtime (a new read
route and a CORS surface for something the build already knows).
- **The ceiling ships as an offered example, not as a hidden one.** A reader who
taps `🍕*🦅` gets 🐧 penguin and a line saying the flag is unreachable and why.
Hiding it would make the plate lie by omission about the genre it is selling.
- **The local browser check does not widen the zone's production CORS.** The
dev server proxies a same-origin path to the local zone and the plate's existing
`?zone=` override points at it, so playing locally needs no new allowed origin.

### D8. Mixing operators is refused

A name that mixes `*` with `+`/`-` is unparseable. The grammar has no precedence
today, and inventing one would make `a*b+c` mean something a caller cannot
predict; the two paths also need disjoint inputs (the term vectors vs. one
composed vector).

*Alternative rejected:* `*` binds tighter and its winning entry's vector feeds
the arithmetic. Rejected because the joint answer is an *entry*, not a
direction: substituting it into `+ c` would answer a question nobody asked, and
silently.

## Risks / Trade-offs

- **Adding 262 entries moves every pinned score** → re-measure `examples.json`,
  the plate copy, and the metadata in the same commit; the drift gate shows the
  diff, and `verified_against` records the new model and entry count.
- **A conjunction can look confident when it should not** → every result carries
  its joint score and the surfaces show it (spec: *A conjunction reports how much
  its terms actually agree*); the weak case is pinned as the ceiling example.
- **Product ranking can favour a broad entry that is mildly similar to
  everything** → measured over the pairs above, the winners are specific
  (panda, dango, stadium, om); the pinned scores make drift visible, and RRF
  remains the documented alternative if it becomes a problem (it measured worse
  here).
- **Flags are near-orthogonal to the rest (legs ≤ 0.36)** → they cannot crowd a
  non-flag answer; the gate re-measures every pinned example after they join.
- **`*` is also the DNS wildcard character** → the zone publishes no wildcard
  records and answers names synthetically; a lone `*` label now refuses as a
  termless name, which is a spec'd scenario and cannot be cached as data.
- **A conjunction's answer is one entry, so `top_k` still lists runners-up** →
  the spec keeps the existing ranked-list contract; the runner-up list is where a
  conjunction's alternatives are visible.
- **A composed query could be one the zone refuses** → tap-to-compose selects
  operators through the plate's own toggle, so a mixed name is not reachable
  from the controls; the free-text input can still produce one, and the plate
  already states a refusal instead of rendering an empty result.
- **Inlining the examples means a rebuild to change a control** → that is the
  point: the change is a data commit with a verification run
  (`just verify-emoji`) rather than a page edit nobody measured.

## Migration Plan

1. Land the grammar and ranking with tests; no behavior changes for `+`/`-`.
2. Rebuild the vocabulary asset with flags, run `just emoji-vocab-check`, then
   re-run `just verify-emoji` to see every moved score before re-pinning.
3. Re-pin `examples.json` (including the ceiling example), inline it into the
   plate, update the plate copy and `docs/dns.md`, and re-run the gate.
4. Check the play surface in a browser against a local zone
   (`just website-dev` runs the site, bridge, and `emb` together).
5. Deploy the zone by hand (`just sandbox-deploy` is for the site sandbox; the
   zone is deployed from `dns/`), since a merged push does not reach the zone.

Rollback: `*` is additive; dropping the flag entries and restoring
`examples.json` restores the previous answers exactly, because `+`/`-` and the
preset are untouched.

## Open Questions

- None. The discovery floors were the one tunable, and the tool's first runs
  settled them at ceiling 0.35, floor 0.28, rare-word 30 (§ D5); changing them
  changes what the search reports, not what the zone answers.
