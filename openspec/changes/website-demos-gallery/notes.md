# Implementation notes — website-demos-gallery

Measurements and review decisions the tasks ask to record, kept beside the
change so the numbers are not folklore.

## Corpus — 4.4 spot-check

`website/tools/demos/fetch-poe.py` → `poe.jsonl`:

```
2 782 passages · 86 works · 392 799 words · sha256:85ee314903bb · 3 040 651 bytes
words per passage:  max 160 · mean 141
bytes per passage:  max 1 140 (bridge cap 2 048)
years:              1827–1849
```

Sampled and read:

| Work | Passages | First passage | Last passage |
|---|---|---|---|
| The Tell-Tale Heart (1843) | 14 | `True!—nervous—very, very dreadfully nervous…` | `…the beating of his hideous heart!”` |
| The Cask of Amontillado (1846) | 16 | `The thousand injuries of Fortunato…` | `…In pace requiescat!` |
| The Raven (1845) | 8 | `Once upon a midnight dreary…` | `…Shall be lifted—nevermore! Published 1845.` |
| Annabel Lee (1849) | 2 | `It was many and many a year ago…` | `…In her tomb by the side of the sea. 1849.` |
| The Purloined Letter (1844) | 48 | `Nil sapientiæ odiosius acumine nimio.— Seneca.` | `They are to be found in Crébillon’s ‘Atrée.’”` |

No work begins or ends mid-sentence; no passage carries boilerplate (asserted by
the tool against seven markers); every id is unique.

**Known, bounded artifacts, accepted:**

- **73 of 2 782 passages (2.6 %) begin lowercase.** Poe writes sentences over
  160 words, and a sentence boundary cannot bound a passage alone, so
  `split_long` breaks at the sentence's own clause punctuation (`;`, `:`, `—`,
  or `,`) and the continuation starts the next passage. They read as
  continuations, never as headless fragments, and only exist because the
  alternative is a 308-word passage.
- **Some dialogue tags open a passage** (`said Legrand, “but it’s so long…`).
  That is naive sentence splitting around a closing quotation mark, not a lost
  paragraph break; the speaker's own words are intact in the previous passage.
- **Verse is reflowed to prose.** A stanza becomes one run of lines so a
  passage is one retrievable unit; the plate carries the poem's work and year,
  which is what identifies it. Preserving line breaks would need a verse
  detector, and the atlas measures passages, not typography.
- **Emphasis underscores are delimiters, not characters.** Gutenberg writes
  `_italic_` and drops a space where the italic abuts the text
  (`the _appearance_of that truth`); naively deleting them welds words
  (`appearanceof`, `aMaison`). An underscore joining two word characters
  becomes the space it stands for, the rest vanish.
- **One source erratum is corrected** (`Wherethe` → `Where the`, *The City in
  the Sea*): listed in `ERRATA`, audit-able against the source volume, and the
  only entry.

## Sandbox — 1.3 / 2.2 / 2.3

_Pending: requires a deploy._

## Index — 5.3

`website/assets/demo/poe-85ee3149.db` · `poe-85ee3149.json`

```
2 782 passages · 86 works · int8 · sqlite-vec v0.1.6 (native build)
minilm    384 dim · recall@10 0.954   bge-small 384 dim · recall@10 0.949
```

`recall@10` is measured, not claimed: 37 probe queries (five written ones plus
thematic queries below, plus 32 passages sampled across the corpus) are ranked
by the int8 `vec0` index and by an exact float32 search over the same vectors.
The corpus is L2-normalized, so the int8 index's L2 distance ranks the same way
cosine does.

`vec0` top-k for the page's own queries (`k = 5`, nearest first):

| Query | `minilm` | `bge-small` |
|---|---|---|
| a guilty conscience that will not stay buried | Marie Rogêt · The Black Cat · The Imp of the Perverse | Marie Rogêt · The Murders in the Rue Morgue · The Purloined Letter |
| a ship in a storm | MS. Found in a Bottle · The Balloon-Hoax · Pym · A Descent into the Maelström · The Oblong Box | Pym · Pym · The Oblong Box · Pym · A Descent into the Maelström |
| the dead returning | Ligeia · Marie Rogêt · The Premature Burial · The Premature Burial | The Premature Burial · Marie Rogêt · The Premature Burial · The Colloquy of Monos and Una |
| a house that remembers its dead | The Black Cat · The Premature Burial · The Fall of the House of Usher | William Wilson · The Murders in the Rue Morgue · The Colloquy of Monos and Una · Usher |
| a man who can read another's mind | The Murders in the Rue Morgue · Pfaall · Loss of Breath | Mesmeric Revelation · Bon-Bon · The Murders in the Rue Morgue |

Every query is phrased in words the corpus does not contain, and every ranking
lands on the works it should. The two models' top-3 sets overlap 0/3, 1/3 and
1/3 for the first three queries: the same corpus, two incompatible spaces.

**Regions.** k-means runs over the embeddings with their mean removed, not over
the 2D projection. Clustering the projection grouped *stories* (Pym appears in
every region) because a two-component projection keeps document length and work
identity; removing the shared component — every passage is one author's English
prose — is what makes the regions themes. `--report-regions` prints each
region's passages, and the names in `CLUSTER_LABELS` come from reading them:

```
minilm    THE DESCENT · THE AIR · THE BRIDE · DETECTION · THE LITERATI ·
          THE METAPHYSIC · THE DREAM · THE CRIME · THE PIT · THE SEA
bge-small THE TREATISE · THE AIR · THE DREAM · THE PURSUIT · THE PLAGUE ·
          THE METAPHYSIC · THE SEA · THE TRICK · DETECTION · THE WHIRLPOOL
```

**Two `vec0` tables, one corpus.** `5.4` is the same file: `vec_minilm` and
`vec_bge_small` over the same 2 782 rowids, and the same query returns different
neighbours from each — the incomparability the model-lens plate demonstrates.

**Naming.** The `.db` and its manifest are named `poe-<corpus sha256[:8]>`, so a
changed corpus cannot be served stale. Verified by rebuilding against a corpus
with one passage removed: `poe-85ee3149.db` → `poe-05acb99c.db`, with the
previous pair deleted rather than left beside it.

## Presets — 3.4 / 3.5

All four digests are stamped from the shipped bytes (`just website-presets
--check`: 11 markers, 4 digests). The rerank's bound is the **bridge's** and the
**server's**: `website/repl/limits.go` charges one work unit per declared text
and refuses more than `max_texts: 8`, and `sandbox.yaml` sets the server's own
`max_texts: 8`, so `rank.lua` and `between.lua` can never be handed more than
eight texts. That is why a rerank carries at most seven candidates, and the
search plate says so on the page rather than raising either limit.

## Client — 6.1 to 6.5

One measurement that shaped the client: the `VALUES` reply is the server's typed
tensor (`dtype`, `shape`, `values`) and the `BLOB` reply is 1 536 packed bytes.
The client embeds a query through `VALUES` — the contract the rest of the site
documents — and decodes a `BLOB` only where a plate is *showing* the bytes (the
vector and the model-is-a-function plates), where the packed form is the point.

Two defects were found by running it, not by reading it, and both are fixed:

- **The `VALUES` reply is not a list of numbers.** It is a flat field/value
  array, so the client reads it by field name. Reading positionally would have
  broken the moment the envelope grew a field.
- **A scripted reply's hash arrives as a flat array** (RESP2 has no map), so
  `preset()` pairs field/value back up. The *number of texts* is the only
  reliable discriminator between "one hash" and "a list of hashes": both are
  arrays on the wire, and guessing from the elements reads a one-hash reply as a
  list of its own fields.

The wasm initialiser is parked on `window` rather than in the module, because
sqlite3's build warns and can wedge if `sqlite3InitModule` is called again while
an initialisation is in flight, and a page can hold more than one instance of
this module.

`prefers-reduced-motion` is read where a plate animates (`motionAllowed()`); the
stylesheet collapses the CSS side, and `verified: no plate animates without
being asked, and no result depends on an animation having run` — the lens's
morph is skipped and the atlas's query is drawn in one frame when it is set.

## Plates — 7.9 to 7.12

- **7.9 — five sections, fixed order, identical headings.** Checked across all
  six plates by parsing the served pages: `WHAT YOU ARE LOOKING AT`, `TRY IT`,
  `WHAT JUST HAPPENED`, `WHY IT MATTERS`, `THE EXACT COMMANDS`, in that order on
  every one. The commands are built at run time from the argv the plate sends.
- **7.10 — the instrument rules.** The gallery's stylesheet block declares **no
  custom property** and one colour literal (`#111110`, the dark ground
  `.block--dark` already declares); every other value is an existing token. No
  new font, texture, card, gradient, rounded panel or shadow. One signal per
  plate: the query, the highlighted neighbours or the similarity.
- **7.11 — the raven: cut.** No mark was drawn. Every plate carries its
  identity from the composition — the numbered title, the ruled figure caption,
  the mono annotations, the quotations — and reads complete without a
  silhouette. The design's own default was to cut it if it read as costume, and
  against these plates it would have been the only piece of decoration on the
  page that says nothing.
- **7.12 — the floors.** `just website-ink` over the gallery: *"PASS — no text
  ink outside the viewport at any of 24 widths"*, which includes the 1086px
  reference frame and 390px. Type floors, target floors and contrast come from
  the shared stylesheet, which the landing and docs are already held to; the
  gallery adds no rule that computes a size or a colour of its own.

## Bookkeeping — 8.1 to 8.4

- **8.1** `published-tree.py` passes: 28 served paths, one origin, the sandbox
  service still excluded. **Demos is in the menu on all ten views** — the
  landing, the docs surface, the not-found page and all seven gallery pages
  carry `Docs · Demos · GitHub`, with the gallery's own links relative
  (`vector.html`, `../docs/index.html`) and the 404's absolute by necessity,
  since it is served at arbitrary paths. All 104 internal references across the
  ten views answer 200 on a live server. The index is the one entry the served set cannot hold
  as a constant, so the check **reads the manifest** — the manifest names the
  hashed `.db`, and the check fails if that file is missing. A rebuild renames
  the index without editing the check; a manifest pointing nowhere still fails.
- **8.2** `websiteDeps` gains `pkgs.sqlite-vec`; the dev shell exports
  `SQLITE_VEC_LIB`, `SQLITE_WASM_VEC_TARBALL` and `SQLITE_WASM_VEC_VERSION`. The
  wasm pin (0.1.9) is one release ahead of the pinned nixpkgs' native extension
  (0.1.6) because npm's oldest `sqlite-wasm-vec` is 0.1.7. That direction is the
  one that matters — the build writes, the browser reads — and it is **checked**:
  a `vec0` int8 table written by 0.1.6 opens and answers a k-NN query under
  0.1.9. The flake comment says so and says what to re-check on a bump.
- **8.3** A gallery change is under `website/`, so `ci.yml`'s existing prefix
  test already leaves the server and gem jobs skipped while the `site` job runs.
  The site job gains one step — `fetch-poe.py --verify` — which re-asserts the
  committed corpus's invariants (unique ids, every passage inside the bridge's
  2 KiB text cap, no acquisition boilerplate) without the network.
- **8.5 (added in review)** `published-tree.py` now also asserts that **every
  internal reference on a served page resolves to a served file**. It was asked
  for and it caught two real classes of mistake on the way in: a reference that
  resolves nowhere (`demos/vectorx.html`), and a same-directory link written as
  an absolute path. Pretty URLs (`/`, `/docs/`, `/demos/`) resolve to the
  directory's own index, and the check strips `<script>`/`<style>` first so a
  plate's own `data-src="embed"` hook is not mistaken for a reference. Verified
  both ways: it fails on a broken plate link and passes on the real tree.
- **8.4** `_headers` pins `/assets/vendor/*` and `/assets/demo/poe-*.db`
  immutable, because both carry their identity in the name, and revalidates
  `/assets/demo/manifest.json`. `published-tree.py`'s unhashed-path check now
  covers `.json` as well, so pinning the manifest immutable would fail.

## Verification — 10.1 to 10.4

- **10.1** `just website-published`, `just website-presets`,
  `just website-ink` (gallery, landing, docs, 404) and `just website-shot` all
  pass; no text ink outside the viewport at any of the 24 widths tested.
- **10.2** `impeccable detect --json` over the seven pages, `demos.js` and
  `styles.css`: **112 findings on the first pass, 105 after the fixes
  below** — against a **39-finding baseline** for the three existing pages at
  the same family level. The families it reports (`cramped-padding`,
  `clipped-overflow-container`, `overused-font`, `wide-tracking`) are the
  incumbent design system's own character, present on the landing and docs too,
  and `product-site` requires the gallery to be built from those atoms rather
  than to fork a second visual language.

  Resolved rather than recorded:
  - **hero-eyebrow-chip: 7 → 0.** The plate number moved from a tracked-caps
    line above the headline into the headline (`II. The atlas`).
  - **Figure captions** no longer uppercase their values: a model name is a
    proper noun and the design's own caption example writes it that way.
  - **Prose em-dashes trimmed** on the plates (15 → 12 on the search, 12 → 10 on
    the function plate) where the dash was decorative.

  Recorded, with the reason: the remaining findings are advisory or inherited —
  `overused-font` names Inter, which is the site's committed face;
  `all-caps-body` counts the gallery's annotation voice (`FIG. · · ·` captions
  and the mono caps the poster already uses for `.cap__label`,
  `.examples__label` and `.plate-label`); `cramped-padding` and
  `clipped-overflow-container` fire on the same `.code` and `html`/`body` rules
  the existing surfaces use; `em-dash-overuse` is advisory and the site's copy
  voice uses em-dashes.
- **10.3** `just sandbox-test` passes; `bridge.go`, `allowlist.go` and
  `terminal.js` are byte-identical to `HEAD`; the refuse list still names
  config, monitor, `EMB.SAVE`, `EMB.CACHE.FLUSH`, `EMB.SCRIPT*`, `EMB.EVAL` and
  `EMB.IMG*`. The only `repl/` changes are `sandbox.yaml` and the two new
  presets. `go vet ./...` and `go test ./internal/registry/ ./internal/config/`
  pass with the one `downloadModel` fix.
- **10.4** Every plate carries its five sections, its scripts, its commands and
  a `<noscript>` note stating that the interactive part needs scripting; the
  live controls are marked `data-live` and hidden by a rule *inside*
  `<noscript>`, so a reader without scripting sees a complete explanation and no
  dead control. With the sandbox unreachable (a dev server pointed at a closed
  port) the plate shows `unavailable — the sandbox could not be reached` with a
  retry, no vector and no ranking, while the explanation, the mechanism, the
  scripts and the captions — whose figures come from the shipped index, not the
  server — stay readable.

## Atlas corrections — 11.1 to 11.5

Both defects were found by running the plate, not by reading it:

- **11.1 — regions under the year order.** `place()` maps `x` from `p.year`
  under the year order, but a region's position is a pair of projection
  coordinates. With `r.year = 0`, every ring computed `cx ≈ −77 364` and a
  **zero radius**, so ten labels stacked off the left edge of the plate. The
  regions are now drawn only by meaning; a cluster is a fact about the space and
  a chronology has none.
- **11.2 — the query under the year order.** The landing's centroid carried
  `{x, y}` and no year, so the year order wrote `cx="NaN"` and the query's mark
  disappeared. The centroid now carries the **mean year of the retrieved
  neighbours**, and the query stands where the passages it found stand. Verified:
  meaning `cx = 441.5`, year `cx = 646.5`, both finite and the readout's layout
  line correct.
- **11.3 — switching the order re-queried.** The switch called `draw(); run();`,
  spending a sandbox round trip (and a token from the 2/s bucket) to recompute a
  centroid it already had. It now redraws from the held hit set and centroid;
  verified by observation that the switch makes no request.
- **11.4 — failures disguised as emptiness.** The blend cleared its result area
  and reported `ready` on failure. It now states the condition, offers a retry,
  and shows no passage, matching the new `embedding-demos` requirement.
- **11.5 — the year order read every passage.** The init fetched all 2 782 full
  passages (text included) to recover their years. `demos.js` gained `years()`,
  which selects `rowid, year` alone.

`just website-ink` caught two code-block regressions the corrections exposed, both
fixed by breaking the long lines: the atlas's projection `SELECT` was 0.88 px over
at 390 px, and the cache plate's aligned comment was 11.88 px over at 320 px. The
probe gained a general `?target=<path>` so a single plate can be swept
(`just website-ink http://localhost:8080 demos/batch.html`); all four surfaces —
landing, docs, 404, and each gallery page — pass at all 24 widths.

## Cost plates — 12.1 to 12.6

The first draft of the batch plate was **wrong**, and the number it printed is the
reason the trap is written down here. It embedded the same six texts on both
sides, so the batched call read the six single calls' cache entries and reported a
**297×** speed-up (72 µs against 21 399 µs) that was almost entirely the cache.
The plate now salts each side with a fresh per-run marker, so neither side shares
an entry:

```
one call   7 275 µs server (execution)
six calls 22 041 µs server (execution)   -> 3.0x less execution time
```

Both figures are the bridge's `elapsed_us`, bracketed around the upstream
command on its loopback connection — the server's execution time, not the
reader's round trip. The plate no longer prints a client wall clock at all: it
would track the network between the reader and the sandbox rather than the work
`emb` did, and the whole point of the plate is the work. `cache.html` reports its
two `elapsed_us` values the same way and says "network excluded" on the line.

The batch gain is real but modest — a six-sequence pass costs about twice a
single pass, not six times — and the plate draws whatever it measures rather than
a figure from a README.

The cache plate reads `EMB.INFO`'s per-model counters around a repeated call, so
it never has to guess its own state. Cold passage on the dev machine:

```
first ask  4 190 µs — miss, and it filled the entry
second ask   103 µs — a hit, served without the model
cache moved +1 hit · +1 miss
```

Warm passage (the same line asked by an earlier reader) reports `+2 hits · +0
misses` and says the first was already cached, rather than presenting it as a slow
miss. `INFO cache` independently reports the server's overall rate (58.6 % at the
time of writing).

Both plates build their bars from the atlas's SVG atoms — `.atlas__svg`,
`.atlas__frame`, `.atlas__mark` and its `is-hit` accent — so the two new pages add
no colour, font, texture, or stylesheet rule. The accent is the winning side on
the batch and the cached call on the cache.

`published-tree.py` now serves 30 paths (from 28): the two plates are in `SERVED`
and `PAGES`, and every internal reference resolves. `just website-presets-check`
still reports 11 markers and 4 current digests, unaffected by the additions.

## Visual first — 14.1 to 14.4

The vector plate now draws the vector: 384 thin bars above and below a centre
line, scaled to the largest magnitude, with one accent on the largest value. A
live run reports `dim 384 · norm 0.99999998 · 1536 bytes` and draws exactly 384
bars and 1 accent from the reply's own floats. The batch, cache, image, and graph
figures all reuse the atlas's SVG atoms; the only stylesheet additions are two
small rules in the gallery block (`.atlas__grid.is-hot`, the image plate's
`.drop`) built from existing tokens, and no new custom property.

`just website-ink` passes at all 24 widths for the vector, image, and graph
plates, including 1086px and 390px.

## Visual embeddings — 15.1 to 15.6

**The model.** `Xenova/clip-vit-base-patch32` int8 is 153 695 702 bytes. Its
fused graph declares `input_ids`, `pixel_values`, `attention_mask` and outputs
`logits_per_image`, `logits_per_text`, `text_embeds`, `image_embeds`. The direct
`EMB` path cannot feed it (the fused graph demands every input for either
branch), which is exactly why the sandbox uses a script:

- `zeroshot.lua` calls `emb.image.preprocess(KEYS[1])`, builds the unused branch
  host-side (`fill = 0`, never a 150k-element Lua table), asks each run for one
  output, and reduces packed float32 with `emb.similarity` and
  `emb.math.softmax`. Eight labels is its own cap.
- A local run over `website/assets/img/og.png` returns a distribution; the
  brownish poster is the plate's own drawn pattern, labelled `chart or diagram`.

**The transport.** `execRequest` gained `bin []int`; `decodeBinary` base64-decodes
those indices and `isImageCall` admits the transport only for the sandbox's
`zeroshot` digest. Bounds: `MaxImages: 2`, `MaxImageBytes: 256 KiB` in the
bridge, `max_images: 2`, `max_image_bytes: 262144`, `max_image_pixels: 1048576`
in `sandbox.yaml`. A decoded image is exempt from the 2 KiB text cap and bounded
by the image cap instead. Tests cover the text-preset refusal, the raw-command
refusal, the count cap, the byte cap, malformed base64, and an out-of-range
index; `EMB.IMG`/`EMB.IMGMULTI` are still refused.

**The page** downscales to a 512px long edge and re-encodes to JPEG before
sending — a drawn pattern left as 8.1 KB on the wire. The bytes answer one
request and are not stored.

**The gate.** The model is ~154 MB of weights, the single biggest addition; it is
phase C, still to be measured on the deployed machine (task 15.7). The gallery
skips the plate if the machine cannot hold it, rather than the measurement.

## The graph — 16.1 to 16.5

`graph.lua` embeds its whole batch with one `emb.embed(KEYS, { bytes = true })`,
computes the N×N cosine matrix in the server's process, and returns one value per
key: `{from, edges = {{to, score}, …}, query}`. Two edges per node, self excluded
by a `-1` sentinel. The matrix never crosses the wire.

`graph.html` seeds eight corpus passages from a query, calls the preset by digest,
and draws the result as a directed graph — nodes on a ring, one arrow per edge,
the strongest edge the accent. A live run over the `the sea` seed reports `8
nodes · 16 edges · strongest 1→2 0.672`, with the works (`Pym`, `Maelström`)
listed below.

The page reads each node's **nested** edge arrays back through `pairsToObject`
(the wire is a flat pair list at every level); the first draft read them raw and
printed `undefined→undefined NaN`. `pairsToObject` is now exported for exactly
that reason.

## Expansion verification — 17.1 to 17.4

`published-tree: ok (32 served paths)`, `stamp-presets: ok (13 markers, 6
digests)`, `openspec validate --strict: valid`, `go build ./...`, `go vet`, and
`go test ./website/repl/` all pass. The gallery is now ten plates across four
tiers: I the vector and the similarity; II the search and the atlas; III the
lens, the function, the image, the batch, the cache; IV the graph.

## The model lens, samples, and motion — 18.1 to 18.7

**The lens rendered nothing.** `render()` built each layout as
`Object.assign({}, la, { bounds: fit(la.points) })`, but `at()` reads
`layout.x0/x1/y0/y1`. Every coordinate computed from `undefined`, so every mark
was `cx="NaN"` and the plate was blank. The fix spreads the fit into the layout
(`Object.assign({}, la, fit(la.points))`); verified: 2 782 marks render, and the
two models' top-5 share 0 neighbours for the default query.

Two more lens defects were fixed while there: the retrieved neighbours were
computed into `layout.hit` but never read, and the morph relied on a CSS
transition of SVG `cx`/`cy`. The neighbours are now lit on the current
projection, and the morph uses the shared `tween` primitive.

**The reduced-motion trap.** The first morph draft borrowed the previous
position whenever `animate` was true and only animated the move when
`motionAllowed()` was true — so a reader who prefers reduced motion got the
*stale* map, not a cut. The fix is to borrow the old position only when it will
actually be animated from. The image plate's bars had the same shape of bug
(widths started at 0 and only the animated branch set them); under reduced motion
they rendered at width 0. Both now go through `tween`, whose reduced-motion path
runs `step(1)` once.

`tween(ms, step)` is the gallery's one animation primitive: the waveform rises
from the centre line, the graph's edges extend and its nodes open, the bars grow,
and the atlas's query lands with one ripple. Every end state is identical with
motion off.

**The samples.** The image plate no longer accepts an upload. It ships six
public-domain images (Doré's raven plate and *Raven* engraving, Aivazovsky's
*Ninth Wave*, the Hartshorn Poe daguerreotype, Fitz Henry Lane's harbour, Van
Gogh's *Sunflowers*), all at a 512px long edge (~44–97 KB each), with a credits
note beside them that is excluded from the served tree. A live run labels the
raven `raven` (0.2405) and the seascape `storm at sea` (0.2728), discriminating
well enough to be convincing.

`published-tree: ok (38 served paths)` — 32 plus the six samples.
`just website-ink` passes at all 24 widths for the index, vector, lens, graph,
image, atlas, batch, and cache plates, and `openspec validate --strict` is valid.

**The flat distribution.** The first `zeroshot.lua` softmaxed the raw cosines,
so eight labels came back at 12.1–12.9 % and the winner was invisible. CLIP is
trained with a learned logit scale (≈100), and the scale is what turns a
similarity into a distribution with a winner; the script now applies it before
the softmax (`emb.math.scale(scores, 100)` → `emb.math.softmax`). The raven
sample goes to `raven 51.7 %` and the seascape to `storm at sea 95.0 %`, so the
chart reads as an answer instead of a tie.
