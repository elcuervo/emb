## Context

See `proposal.md` — Why. The current state that shapes the approach:

- The site is three static surfaces — `website/index.html`, `website/docs/index.html`,
  `website/404.html` — served from a Cloudflare Worker with no build step.
  `website/tools/published-tree.py` asserts the exact served set, and
  `website/.assetsignore` is the only thing between a file and a public URL.
- `website/repl/` is a Go bridge that is the sandbox's only public surface. It
  builds with `CGO_ENABLED=0`, holds one upstream RESP connection, and accepts
  one endpoint — `POST /api/exec {args, proto}` — validated against
  `website/repl/allowlist.go`. `EMB`, `EMB.MULTI`, `EMB.MODELS`, `EMB.INFO`,
  `EMB.EVSHA` (preloaded digests), `INFO`, `PING`, `HELLO` are permitted;
  `CONFIG`, `MONITOR`, `EMB.SAVE`, `EMB.CACHE.FLUSH`, `EMB.SCRIPT*`, `EMB.EVAL`
  and `EMB.IMG*` are refused.
- The sandbox machine is `shared-cpu-1x · 2 GB` with a 3 GB volume, always on.
  Measured steady RSS is **1.40 GiB** with `minilm` and `sst2` at fp32 — roughly
  **0.45 GiB of headroom**.
- `emb` already implements `quantize: auto|on|off` per model
  (`internal/config`, `internal/registry`), and `internal/hfhub` already resolves
  `onnx/model_quantized.onnx`. `EMB.INFO` already reports `quantization`.
- `emb`'s script surface (`docs/scripting.md`, `internal/script`) exposes
  `emb.embed`, `emb.similarity` / `emb.distance`, `emb.math.{dot,cosine,l2,norm,topk,softmax,argmax}`,
  `emb.tokenize.encode` / `encode_pair`, and `emb.run` / `emb.run_batch`, all
  sharing the server's batcher, cache and sessions.
- The sandbox preloads `presets/embed.lua` and `presets/classify.lua` and accepts
  `EMB.EVSHA <model> <sha> <n> <text…> <arg…>` for exactly those digests
  (`allowlist.go`). The bridge caps a request at `max_texts: 8`.
- The site's design language is committed in `DESIGN.md` and restated as
  requirements in `product-site`: paper `#F3F0E8`, dark `#111110`, one accent
  (`#FF5A1F` surface only; `--accent-ink` for text/rings), a 12 px / 14 px type
  floor, a 44 px target floor, `prefers-reduced-motion`, no gradients, cards,
  rounded panels, or shadows. Its visual vocabulary is a technical poster with a
  signal spine, isometric plates, a ruled ledger, and an engraved terrain.
- The published tree serves a content-hashed binary already
  (`assets/cast/emb-top-dd60083b.cast`), and `assets/js/asciinema-player-3.17.0.min.js`
  is a vendored, version-pinned third-party build fetched by `flake.nix` and
  re-vendored by `just website-player`. Both are the precedent this change
  follows.

## Goals / Non-Goals

**Goals:**

- A reader who has never seen a vector can, in one plate and one click, watch text
  become a vector and watch that vector retrieve by meaning.
- The gallery shows, visibly, that two embedding models produce two
  incomparable spaces — the fact that makes "multiple embeddings" a real decision.
- The whole pipeline is real: the sandbox's own models embed the corpus, a real
  index answers the query, the scripts are the sandbox's own preloaded bytes, and
  the commands shown are the commands that ran.
- The gallery reads as one instrument — an engraved atlas — rather than seven
  unrelated pages, without adding a colour, a font, or a decoration.
- Zero added Fly cost: no new machine, endpoint, dependency, or egress.
- The sandbox's read-only posture, its spec surface, and its bounds are unchanged.

**Non-Goals:**

- **Image embedding is enabled, with limits, not deferred.** A fused CLIP
export (int8, ~154 MB) joins the model set and one scripted preset scores an
image against labels. The earlier deferral is reversed because the wow is
worth it and the risk is boundable: the browser downscales before sending, the
bridge carries binary only for that one preloaded digest, the request is capped
at two images, 256 KiB each, and one megapixel, and nothing is stored. The
remaining risk is memory, and the model is gated on the same RSS measurement as
the dimension dial (D3, D14).
- **A server-side vector store.** No pgvector, no Qdrant, no sqlite-vec in the
  bridge. A demo corpus is small and read-only; a networked store buys nothing a
  client-side index does not, and costs a machine.
- **Visitor-authored corpora.** No upload, no "add your document", no stored
  embedding, no account.
- **Raw Lua.** `EMB.EVAL` stays refused. The sandbox runs scripts it preloaded by
  digest; the demo shows that boundary rather than eroding it.
- **A cross-encoder reranker.** `bge-reranker-base` int8 is ~280 MB and would be
  its own model. The rerank demo uses embedding cosine in Lua, which needs no new
  model and is honest about what it is.
- No change to the bridge, the allowlist, the presets' *mechanism*, the reply
  contract, or `terminal.js`'s renderer.
- Not retiring `sst2`. It is not an embedder and that is exactly why it stays: it
  is the "not every model returns a vector" contrast, and the fourth answer in
  the model-is-a-function plate.

## Decisions

### D1. The corpus is Poe, and it is a passage-level atlas

Edgar Allan Poe's tales and poems, public domain in every jurisdiction, fetched
from Project Gutenberg plain-text editions. Chosen over a mixed or populist
corpus because a single author's collected work gives the map *real continents*
without curation: the sea stories, the detection stories, the confessional
madness stories and the elegies occupy distinct, stable regions, and a reader
recognises them instantly.

- **Works**: the major tales and poems — *The Tell-Tale Heart*, *The Fall of the
  House of Usher*, *The Cask of Amontillado*, *The Pit and the Pendulum*, *The
  Black Cat*, *The Masque of the Red Death*, *Ligeia*, *Berenice*, *Morella*,
  *William Wilson*, *The Oval Portrait*, *MS. Found in a Bottle*, *A Descent into
  the Maelström*, the three Dupin stories, *The Gold-Bug*, *The Facts in the Case
  of M. Valdemar*, *The Premature Burial*, *The Imp of the Perverse*, *The Raven*,
  *Annabel Lee*, *Ulalume*, *The City in the Sea*, and others.
- **Unit**: the passage. Paragraphs are split at sentence boundaries to at most
  ~160 words, and fragments under ~8 words are dropped. Each passage carries its
  work, its year, its index, and a display snippet.
- **Licence**: public domain. The Gutenberg boilerplate is stripped and the
  trademark is not reproduced; the page carries one attribution line — *Texts:
  Edgar Allan Poe (1809–1849), public domain, via Project Gutenberg.*
- **Budget**: roughly 2,000–5,000 passages, which at 384 dimensions and int8
  storage is ~1–2 MB on the wire.

### D2. One build tool owns the pipeline: corpus → `emb` → committed `.db`

`website/tools/fetch-poe.py` acquires, strips and chunks the corpus into
`website/tools/demos/poe.jsonl`. `website/tools/build-demo-db.py`:

1. reads the corpus,
2. embeds every passage against a **local** `emb` over RESP, batched, offline,
3. projects the vectors to 2D at build time (PCA), and runs a small k-means over
   the projection to produce cluster regions,
4. writes `website/assets/demo/poe-<sha8>.db` containing one `vec0` table per
   model, a table of passage metadata, the projection, the cluster regions, and
5. writes a manifest (model, dimension, count, precision, source hash, per-table
   recall probe, cluster labels) that the page reads so no number is transcribed.

The corpus, the tools and any intermediate output are excluded from the deployed
tree; only the `.db` and its manifest ship. Cluster regions are **hand-named** at
build time from the computed centroids (`THE SEA`, `THE GRAVE`, `MADNESS`,
`DETECTION`, `GRIEF`, …) and committed in the manifest, which is what makes the
atlas readable rather than a cloud of dots.

### D3. The model set: quantize first, then spend the headroom on demo models

`quantize: auto` on every sandbox model, and the demo models added are the ones
that carry a lesson. Verified artifact sizes (HF API, `model_quantized.onnx`):

| Model | dim | fp32 | int8 | Lesson it carries |
|---|---|---|---|---|
| `all-MiniLM-L6-v2` | 384 | 90.4 MB | **23.0 MB** | baseline space; the atlas, similarity, search |
| `distilbert-sst2` | 2 | 268.0 MB | **67.6 MB** | a model that returns **no vector** |
| `bge-small-en-v1.5` | 384 | 133.1 MB | **34.0 MB** | the lens: same dimension, **different space** |
| `paraphrase-multilingual-MiniLM-L12-v2` | 384 | 470.3 MB | **118.3 MB** | cross-lingual: query in one language, passage in another |
| `all-mpnet-base-v2` | 768 | 435.8 MB | **110.1 MB** | the dimension dial (phase B) |
| CLIP ViT-B/32 (fused, text + vision int8) | 512 | — | **153.7 MB** | the image: one space holds a photo and a sentence |

Budget:

```
today (fp32 pair)                  90 + 268            = 358 MB
phase A (int8 + 2 demo models)     23 + 68 + 34 + 118  = 243 MB   −115 MB
phase B (+ dimension dial)         243 + 110           = 353 MB   ≈ today
deferred phase C (+ CLIP)          353 + 154           = 507 MB   +149 MB → gated on the RSS measurement (D14)
```

- **Why int8:** ~4× less resident weight memory, a smaller first-boot download
  (91 MB vs 358 MB → shorter `/api/ready`, less pressure on the 60 s health
  grace), and typically 1.5–2× faster CPU inference via ORT's QLinear paths.
  `EMB.INFO` already reports `quantization: int8`, so the gallery can *show* its
  own precision instead of claiming it.
- **`max_length` is sized for passages.** The sandbox sets `minilm` to 256
  tokens, which truncates a Poe paragraph. The atlas runs with `max_length: 512`
  on the passage-level models (`minilm` supports 512 positions) and the build
  tool reports the truncated fraction so the number is visible rather than
  guessed.
- **Volume gotcha:** `downloadModel` returns early when `cfg.ONNX` exists, and
  `resolveQuantize` only looks for a *sibling* `model_quantized.onnx`. A deploy
  onto the existing volume would therefore stay fp32. The model paths are bumped
  (`/data/models/int8/<name>/model.onnx`) so the first boot after this change
  downloads the quantized weights deterministically and self-heals.
- **Local dev parity:** `just download-model` hard-codes fp32. A sibling target
  (`just download-model-quantized`) fetches `model_quantized.onnx` so the local
  corpus build uses the same weights the sandbox serves.

### D4. The plates: nine demos in three tiers

| Plate | Title | Model(s) | Corpus | Mechanism | Teaches |
|---|---|---|---|---|---|
| I | **The vector** | `minilm` | none | `EMB … VALUES` | text → 384 floats; `norm = 1`; the bytes |
| I | **The similarity** | `minilm` | none | `EMB.EVSHA embed.lua` | similarity is the primitive; one hash, not 768 floats |
| II | **The search** | `minilm` | atlas | `vec0` top-k → `rank.lua` rerank | retrieval by meaning; the RAG step |
| II | **The atlas** | `minilm` | atlas | build-time projection + cluster regions | vectors have geometry; meaning has continents |
| III | **The model lens** | `minilm` + `bge-small` | atlas | two `vec0` tables, one corpus | same text, different space; **you cannot mix models** |
| III | **The dimension dial** | `minilm` + `mpnet` | atlas | two tables, 384 vs 768 | dimension is a cost/quality dial (gated on D3's measurement) |
| III | **The model is a function** | `minilm` + `sst2` | none | `EMB` vs `EMB.EVSHA` ×2 presets | one primitive, four answers; extensibility without forking |
| III | **The image** | `clip` | none | `EMB.EVSHA zeroshot.lua` over binary | one space holds an image and a sentence; zero-shot tagging |
| III | **The batch** | `minilm` | none | `EMB` ×6 vs `EMB` ×1 carrying six texts | one call, many texts: the batcher amortizes the pass |
| III | **The cache** | `minilm` | none | `EMB.INFO` around `EMB` twice | the second ask is a lookup; the vector is memoized |
| IV | **The graph** | `minilm` | atlas | `EMB.EVSHA graph.lua` over 8 passages | the script builds an N-by-N graph beside the model and returns edges |

The atlas carries one extra instrument, not a plate of its own: an **order
switch** that places the passages by meaning or by year, so the reader can see
that the space tracks the work rather than the clock. The switch is honest about
what each order can place: the named regions are drawn only by meaning, and the
query is placed by year at the mean year of the neighbours it retrieved (D11).

### D5. The gallery is an instrument: the aesthetic, in the existing tokens

The concept is **an engraved atlas of a mind**. Poe's century is the century of
the steel engraving, the phrenological plate and the star chart, and the site's
poster language is already that: ink on warm paper, ruled ledgers, numbered
plates, mono annotations, a signal line, an engraved terrain. The gothic is in
the *subject* — the passages, the work titles, the years — and never in the
decoration.

Rules, all enforceable against the existing stylesheet:

1. **Paper explains, the dark plate instruments.** Each plate mirrors the
   landing: the five teaching sections on paper, the interactive apparatus on
   `#111110`.
2. **One signal, spent on meaning.** Every plate has exactly one coloured thing:
   the query, the highlighted neighbours, the similarity arc, the morphed point
   set. Everything else is `--fg` ink on paper or `--bg` ink on the plate. On
   paper the signal is `--accent-ink`; on the plate it is `--accent`.
3. **The atlas is drawn, not rendered.** Points are small filled marks — a star
   chart, not glowing bubbles. Density reads as stipple. The graticule is a ruled
   grid, the cluster regions are hand-named mono caps, and a few passages are
   labelled at their glyphs.
4. **Every plate is captioned with its real figures.** `FIG. 2 — THE ATLAS · 3,481
   PASSAGES · all-MiniLM-L6-v2 · 384 DIMENSIONS · 1.6 MB`, read from the
   manifest. The caption is the `embedding-demos` requirement that numbers derive
   from the index, worn as the aesthetic.
5. **Passages are quotations.** Display face for the passage, mono caps for
   `THE TELL-TALE HEART · 1843`. Emphasis is weight and rule, never a second hue
   and never italics-as-gothic.
6. **Motion is the signal moving, and it is the reader's choice.** The query
   lands with one short, precise move — no bounce, no ease-out-elastic. The model
   lens's morph is the one place motion *is* the argument. `prefers-reduced-motion`
   collapses all of it to a cut.
7. **The raven is one mark, or none.** A single line-drawn silhouette is permitted
   as the gallery index's mark and is drawn at the wordmark's stroke weight. If it
   reads as costume in review, it is cut — the typography carries the plate.

### D6. The scripting surface is demonstrated, not hidden

`EMB.EVSHA` accepts only the digests `sandbox.yaml` preloaded, and `EMB.EVAL` is
refused. That is the honest surface, and the gallery turns it into the lesson:

- `embed.lua` (existing) returns `{dim, norm, similarity?}` — the similarity plate
  and the search's second opinion.
- `classify.lua` (existing) returns `{label, confidence, scores}` — the fourth
  answer in the model-is-a-function plate.
- `rank.lua` (new) takes a query and up to seven candidate passages and returns
  them ordered by `emb.similarity` — the search plate's rerank, with no new model.
- `between.lua` (new, optional) blends two passages' embeddings and returns the
  nearest corpus items — "the space between".

Every script is shown on the page with its digest beside it. A digest that is not
stamped is a call the sandbox refuses, so the page cannot describe a script the
server did not load — the same discipline `just website-presets` already
enforces.

**Constraint to respect:** the bridge caps a request at `max_texts: 8`, so a
rerank is at most seven candidates. The plates either respect that or the bridge's
`max_texts` moves; this change respects it and says so on the page.

### D7. The only live dependency is `terminal.js`'s own request shape

The gallery's client module (`website/assets/js/demos.js`) uses the request
contract the shared client already owns — `POST /api/exec {args, proto}` with
`VALUES` and with `EMB.EVSHA` — rather than inventing a second way to talk to the
sandbox. It exposes: text in, `Float32Array` out; and a script call by digest,
envelope out. The module captures the sandbox origin the way `terminal.js` does,
so no page hardcodes a host.

No new endpoint, no change to the allowlist, no change to the bridge. `/api/presets`
is the precedent for "the page derives its model list from the server"; here the
model list comes from `EMB.MODELS` through the existing command path.

### D8. Honest degradation reuses the state machine the console already has

Unavailable / starting / capacity map onto the console's existing states. A plate
in any of them states the condition, offers a retry, and shows no result. The
corpus, the atlas, the explanation and the scripts are static markup and stay
readable, and the page is complete with scripting disabled (`embedding-demos`
requires it).

### D9. Optimization ladder

Applied in order of effect per effort:

1. **int8 weights** (D3) — memory, boot, latency; already implemented in `emb`.
2. **Build-time 2D projection and cluster regions** — the atlas costs no runtime
   compute and no runtime dependency.
3. **Browser-side exact search** — the ranking is computed on the reader's
   machine, so the sandbox pays one embedding per query and nothing else; the
   sandbox's work ceiling and rate limits still bound that.
4. **Static, content-hashed, edge-cached assets** — the `.db` and the wasm are
   served by Cloudflare, never by Fly; the hashed filename (`poe-<sha8>.db`)
   mirrors `emb-top-dd60083b.cast` and lets them be cached long-term without
   going stale.
5. **Lazy load** — the wasm and the index are fetched on first interaction with a
   plate, not on page load, so the gallery index stays cheap.
6. **Precompressed assets** — ship `.db` and wasm brotli/gzip variants so the
   transfer is compressed without a build step at the edge.
7. **Fuse the model set** — one `.db`, two tables, one asset for the lens.
8. **`emb`'s own LRU cache** — the sandbox's 64 MB cache holds ~43k vectors, so a
   plate's repeated canned queries are cache hits and cost almost nothing.
9. **Precomputed vectors for the canned examples** — the first click is instant
   even before the live call returns, replaced by the live result when it lands.
10. **Storage precision ladder in `vec0`** — `float32` → `int8` → `bit[N]`
    (binary + Hamming). The atlas ships at whatever precision meets the corpus's
    `recall@k`; the ladder is itself a demo ("a binary index, 32× smaller").
11. **Passage-level chunking** — bounded at ~160 words keeps each vector crisp
    and inside the model's window, which is both a quality and a cost decision.

### D10. Bookkeeping the tree already demands

- `published-tree.py`'s `SERVED` gains the gallery pages, `assets/js/demos.js`,
  the vendored wasm, and the `.db`; `PAGES` gains the gallery pages so each
  declares an absolute canonical and social image; the corpus and the tools are
  excluded (and asserted absent).
- `website/.assetsignore` gains the corpus/tooling exclusions.
- `flake.nix`'s `websiteDeps` gains `sqlite-vec` for the build tool. It is pure C
  and must stay substitutable; the check in `AGENTS.md` applies. If nixpkgs does
  not carry it, the vendored wheel is pinned the way the player is.
- `ci.yml`'s path filter learns `website/demos/` and `website/assets/demo/`, and
  the published-tree check runs in `site.yml` before publish as it does today.

### D11. The atlas's order switch places only what the order can place

The switch is the atlas's one piece of interaction, and it was the one place the
plate could draw a coordinate it did not have. Two defects made it wrong, both
found by running it:

- **Under the year order the region rings collapsed at the origin.** A region's
  position is a pair of **projection** coordinates; the year order mapped `x`
  from a year, so a region with no year computed a coordinate far off the plate
  and a zero radius, stacking ten labels at the axis' edge. The fix is to draw
  the named regions **only** in the meaning order: a cluster is a fact about the
  space, and a chronology has none.
- **Under the year order the query's mark became `NaN`.** The landing placed the
  query at the centroid of its retrieved neighbours, and the centroid carried an
  `x` and a `y` and no year; the year order read `p.year`, got `undefined`, and
  wrote `cx="NaN"`, which draws nothing. The fix is to carry the **mean year** of
  the neighbours on the centroid — the same honest rule as the place: a query has
  no publication year, so it stands where the passages it found stand.

Switching the order no longer re-runs the search, either. The plate holds the
centroid and the hit set, so the switch redraws from data already on the page;
re-querying was both a wasted round trip and a spend against the sandbox's rate
limit. An empty result is now a stated error rather than a divide-by-zero, and
the secondary instrument — the blend — reports its failure instead of clearing
itself to look like an empty success (this is the `embedding-demos` requirement
that a failed interaction is never rendered as an empty result).

### D12. The cost plates measure, and the cache is the trap

The two cost plates exist because the site argues `emb` is fast and never shows
it. Both draw the server's own execution time and nothing the network did:

- **The batch** compares six single `EMB` calls against one `EMB` carrying six
texts and draws the summed and single `elapsed_us` to scale. `elapsed_us` is the
bridge's own bracket around the upstream command on its loopback connection, so
it is execution time and does not grow with the reader's distance; the plate does
not time its own requests, because a client clock would measure the ocean between
the reader and the sandbox. The first draft of it was wrong in a way worth
recording: it embedded
the same six texts on both sides, so the batched call read the six calls' cache
entries and reported a **297×** speed-up that was almost entirely the cache. The
plate now salts each side with a fresh marker, so the two sides share no entry
and the number it draws is the batch's own.
- **The cache** asks for one passage twice with an `EMB.INFO` on each side and
  reports the counter deltas, so a reader sees `+1 miss · +1 hit` on a cold
  passage (`4 190 µs` then `103 µs` on the dev machine) and `+2 hits` on a warm
  one. It never asserts which case it is in; the model's counters decide.

Both reuse the atlas's SVG atoms for the bars — `.atlas__svg`, `.atlas__frame`,
`.atlas__mark` and its `is-hit` accent — so the two new plates add **no** colour,
font, texture, or stylesheet rule. The accent is the winning side on the batch
and the cached call on the cache, which is why the same class does both jobs.

### D13. The gallery is visual first

A gallery that explains vectors in paragraphs repeats the failure it exists to
fix. Every plate now carries a figure built from the live reply:

- **The vector** is a waveform: one thin bar per value, above the centre line
  when positive and below when negative, scaled to the largest magnitude, with
  the single largest value the only accent. A list of 384 numbers hides its own
  shape; the waveform shows it.
- **The graph** is a directed graph: nodes on a ring, one arrow per outgoing
  edge, the strongest edge the accent. The ring is a place to stand, not a
  claim about geometry — the arrows are the only claim.
- **The image** is label-probability bars, and **the batch** and **the cache**
  are measured bars, all on the same SVG atoms the atlas already declared.

Reusing `.atlas__svg`, `.atlas__frame`, `.atlas__mark`, and `.atlas__mark.is-hit`
means four new figures added no colour, font, texture, or stylesheet token. The
prose contract is the other half: a plate's sections become captions of the
figure rather than a substitute for it, and the exact commands stay as they are.

**Motion is the figure arriving.** One primitive, `tween(ms, step)` in
`demos.js`, animates every figure: the waveform rises from the centre line left
to right, the graph's edges extend to their end and its nodes open, the bars
grow, and the atlas's query lands with one ripple while its neighbours flare.
The primitive reads `motionAllowed()` itself, so under `prefers-reduced-motion`
`step(1)` runs once and the figure is drawn complete — the end state is the same
and nothing depends on the animation. The model lens's morph is the one place
motion *is* the argument; it uses the same primitive and becomes a cut.

### D14. Visual embeddings, enabled inside a bounded transport

The highest-wow demo is a picture in the same space as a sentence, and the
security analysis that deferred it is answered rather than ignored:

- **No visitor file.** The plate ships six public-domain samples (Doré,
  Aivazovsky, Hartshorn, Lane, Van Gogh) and scores whichever the reader picks;
  there is no drop target and no file input, so the sandbox never receives an
  arbitrary upload from a stranger. The samples are committed at a 512px long
  edge, and the page still bounds and re-encodes the image before sending it, so
  the cap holds even if a sample is replaced.
- **The transport is explicit.** `execRequest` gains `bin`, a list of argument
  indices that are base64 rather than text; the bridge decodes them back to bytes
  before the command reaches `emb`. Binary is refused unless the call is
  `EMB.EVSHA` naming the sandbox's own `zeroshot` preset by a preloaded digest,
  so the transport cannot be pointed at a text preset or a raw image command.
- **The bounds are doubled.** The bridge caps two images per request and 256 KiB
  each; the server caps `max_images`, `max_image_bytes`, and `max_image_pixels`
  before decode. The browser downscales to 512 px before sending, so a phone
  photo never reaches the sandbox whole — the first and cheapest of the limits.
- **Nothing is stored, and nothing new is raw.** The bytes answer one request.
  `EMB.IMG`/`EMB.IMGMULTI` stay outside the surface; only the sandbox's script
  sees an image, and the model name is the preset's, not the visitor's.
- **The fused graph is why the script exists.** The CLIP export demands both
  branches' inputs for either run, so `zeroshot.lua` builds the unused branch as
  a constant tensor host-side (`fill = 0`, no 150k-element Lua table) and asks
  the graph for one output per run.

The model is ~154 MB of weights and is the single biggest memory addition; it is
phase C, gated on the same `EMB.STATS` RSS figure as the dimension dial, and a
machine that cannot hold it skips the plate rather than the measurement.

### D15. The scripting layer computes structure, not just a value

The gallery already showed a script returning a number, a hash, and a label. The
graph plate shows the next step: `graph.lua` embeds its whole batch in one call,
builds an N-by-N cosine matrix, and returns each node's two nearest neighbours —
sixty-four cosines in, sixteen edges out, the matrix never on the wire. It is the
same `emb.embed` batcher and cache as `EMB`, the same `emb.math` reductions that
keep the work host-side, and the page draws what comes back. This is the
capability the product's docs call production scripting, made visible in one
request.

## Risks / Trade-offs

- **Chunking changes what a query retrieves.** → The split is deterministic and
  committed; the manifest records the passage count and the truncated fraction,
  and a retrieval spot-check is a task so a bad split is visible before shipping.
- **Gutenberg boilerplate leaks into the corpus.** → The fetch tool strips the
  header/footer by their markers and asserts that no stripped text remains; the
  attribution line ships on the page.
- **`sqlite-vec` is pre-v1.** → Vendor a pinned build and version the filename
  like the player; a bump is a hash and a re-vendor, contained to one `just`
  target.
- **Page weight** (wasm + `.db`). → Lazy load on first interaction, ship the
  corpus at int8 or `bit[N]` where recall allows, and keep the corpus in the low
  thousands of passages.
- **int8 shifts the vectors slightly.** → Sub-1-point on MTEB and irrelevant to a
  demo; the gallery states the precision via the manifest and `EMB.INFO` rather
  than hiding it, and the fp32/int8 cosine tolerance is already a spec'd
  requirement in `int8-weight-quantization`.
- **500 words that read as a costume.** → The raven is one optional mark and the
  decoration rule is "no new colour, font, texture or shadow"; review cuts the
  mark before it cuts the type. The gothic is in the passages.
- **`max_texts: 8` bounds the rerank.** → The plates respect it and say so; the
  alternative (raising the bridge limit) is named but not taken.
- **Scripts could do more work than they declare.** → Presets are fixed bytes
  owned by the sandbox and `EMB.EVAL` stays refused, which is exactly why the
  work ceiling can safely count declared texts as a proxy.
- **A deploy onto the existing volume keeps fp32.** → The model path bump (D3)
  makes the switch deterministic; rollback is a path revert plus a re-download.
- **Adding models could outgrow 2 GB.** → Measure RSS via `EMB.STATS` after each
  model; phase B and the deferred phase C are gated on that measurement, and the
  fallback is 4 GB at +~$10/mo.
- **A plate cannot reach the sandbox.** → D8; no fabricated result, static content
  intact, retry offered.
- **Abuse.** → No new public surface, no writes, no uploads; a plate's query is one
  embedding, bounded by the limits the sandbox already enforces.

**Cost after this change**

| Item | Δ/month |
|---|---|
| Quantize the pair | $0 |
| `bge-small` + `multilingual` int8 | $0 (covered by the int8 savings) |
| `mpnet` int8 (phase B) | $0 if the measurement holds, else +$10 (4 GB) |
| `clip` int8 (the image plate) | $0 if the measurement holds, else +$10 (4 GB) |
| Browser search + Cloudflare assets | $0 |
| Fly egress | ≈ $0 (corpus never touches Fly) |
| **Total** | **≈ $11.56/mo unchanged** |

## Migration Plan

1. **Sandbox first, behind measurement.** Add `quantize: auto`, bump the model
   paths, add `bge-small` and `multilingual`; deploy with `just sandbox-deploy`;
   confirm `/api/ready` and read RSS from `EMB.STATS`. Decide phase B (the
   dimension dial) and phase C (the `clip` image model) from that number; if it
   does not fit at 2 GB, record the figure and skip the model rather than
   resizing on a guess.
2. **Presets.** Author `rank.lua`, `between.lua`, `graph.lua`, and
   `zeroshot.lua`, list every preset in `sandbox.yaml`, and stamp the digests
   with `just website-presets`.
3. **Corpus.** Run `just website-poe` to fetch, strip and chunk; run
   `just website-demos` to build the `.db` and its manifest against a local `emb`.
4. **Ship the plates.** Land the gallery index and the seven plates, the vendored
   wasm, and the lazy loader.
5. **Bookkeeping.** Extend `published-tree.py`, `.assetsignore`, `flake.nix`,
   `ci.yml`, and the docs.
6. **Verify.** `just website-published`, `just website-presets`,
   `just website-ink`, `just website-shot`; `impeccable detect`; then the site
   deploy's own origin probe.
7. **Rollback.** The site is one commit `wrangler rollback` can revert; the
   sandbox is a config revert plus a model-path revert, after which the next boot
   re-downloads fp32.

## Open Questions

- **Phase B timing.** Whether `mpnet` (the dimension dial) lands in this change or
  the next depends on the RSS measurement in migration step 1; the design covers
  both.
- **The raven.** Whether the single silhouette mark earns its place is a review
  decision with a defined default (cut it).
- **`between.lua`.** Whether the "space between" plate ships or the atlas absorbs
  it as an interaction is a content decision that does not affect the
  architecture.
