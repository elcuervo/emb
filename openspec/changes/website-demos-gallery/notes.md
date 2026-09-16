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
