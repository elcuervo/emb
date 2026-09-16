## Why

The site argues that vectors are useful and never lets a developer *feel* why. The
live console returns a vector, but 384 floats on their own demonstrate nothing,
and nothing on the site answers the question the product's own headline raises —
why anyone would want **more than one** embedding. A developer who does not
already work with vectors leaves the page able to run `EMB` and unable to say
what it is for.

The missing piece is not more copy. `EMB` answers a question, and the answer is
only legible next to the thing that consumes it — a vector index. A gallery of
demos that embeds real text with the real sandbox and searches it with a real
vector store shows the whole pipeline, and a model set that ships several
embedding models lets the gallery show the one fact every newcomer gets wrong:
two models produce two **incompatible** spaces.

## What Changes

- **A demos gallery** at `/demos`: an index plus seven demonstration plates,
  built from the poster's existing atoms. Three tiers — what a vector is (the
  vector, the similarity), why vectors help (the search, the atlas), and why
  `emb` and why several models (the model lens, the dimension dial, the model is
  a function).
- **The gallery is an instrument, not a scrapbook.** It is presented as an
  engraved atlas of a corpus: ruled graticules, numbered plates with monospace
  captions carrying the real figures, passages set as quotations with their work
  and year, and exactly one coloured thing on every plate — the orange signal,
  spent only where meaning is. The gothic is in the *subject*, never in the
  decoration; no new palette, font, gradient, card, or shadow is introduced.
- **The corpus is Edgar Allan Poe's public-domain works** (the "atlas"): the
  tales and poems, fetched from Project Gutenberg, with the boilerplate stripped
  and the text chunked into passages that carry their work and year. A single
  author's collected work is a corpus whose meaning clusters into themes — the
  grave, the sea, madness, detection, grief — which is exactly the structure the
  map exists to reveal, and it is public domain, so it can be committed and
  shipped without a licensing question.
- **A corpus pipeline** (`website/tools/build-demo-db.py`): reads the committed
  corpus, embeds it with `emb`, and writes a committed SQLite database with
  `sqlite-vec` `vec0` tables — including a precomputed 2D projection and
  build-time cluster regions for the atlas. The database is a **static asset**,
  built offline and shipped like any other.
- **sqlite-vec runs in the browser**, vendored and pinned the way the asciinema
  player already is. Search is client-side over the committed database; the only
  live call is one `EMB … VALUES` per query, through the allowlist the sandbox
  already exposes. No bridge change, no new endpoint, no server-side vector
  store, and no additional Fly machine.
- **The demos use the Lua surface, because it is the product's own argument.**
  The similarity, the search's rerank, and a dedicated plate show the same model
  call returning raw bytes, a typed `{dim, norm}`, a `{similarity}`, and a
  `{label, confidence}` — served by preloaded presets called by digest, with the
  script source and its digest on the page. `EMB.EVAL` stays refused: the sandbox
  runs **its** scripts, and demonstrating that boundary is part of the lesson.
- **The sandbox's model set is quantized to int8 and extended with demo models.**
  `quantize: auto` is already implemented and the upstream repos already ship
  `model_quantized.onnx`; turning it on drops the current pair from 358 MB of
  weights to 91 MB. That headroom funds the demo models, so the gallery's model
  set costs less memory than today's pair.
- **Two demo models ship now, one after measurement.** `bge-small-en-v1.5`
  (384-d, retrieval-tuned) is the lens's second space; `paraphrase-multilingual-
  MiniLM-L12-v2` (384-d, 50+ languages) makes the search answer a query in one
  language with a passage in another. `all-mpnet-base-v2` (768-d) adds the
  dimension dial once RSS is measured.
- **The published tree grows a gallery**, its wasm, and its database;
  `published-tree.py`, `.assetsignore`, and the deployment check learn about them.

## Capabilities

### New Capabilities

- `embedding-demos`: the gallery's teaching contract (one demo = five sections,
  fixed order), its presentation contract (an instrument, one signal, figures
  drawn from the index), its corpus pipeline (corpus → `emb` → committed
  `sqlite-vec` database), its browser-side retrieval, its use of the scripting
  surface through preloaded digests, its honest offline behaviour, and the model
  set the demos require.

### Modified Capabilities

- `product-site`: the site gains a third surface and a door to it; the gallery's
  composition is held to the poster's existing atoms and the committed type,
  contrast, and motion floors, exactly as the landing and documentation surfaces
  are.
- `site-deployment`: the published tree's served set is no longer one landing
  page and one documentation surface — it includes the gallery pages, their
  vendored wasm, and their database, and the check that asserts the tree learns
  about each.

## Impact

- **New surface**: `website/demos/` (index and seven plates), `website/assets/js/`
  (a gallery client module), `website/assets/vendor/` (pinned `sqlite-wasm-vec`).
- **New build tooling**: `website/tools/build-demo-db.py`, a corpus-acquisition
  tool (`website/tools/demos/fetch-poe.py`), the committed corpus
  (`website/tools/demos/poe.jsonl`), and a `just website-demos` target.
- **New presets**: `website/repl/presets/rank.lua` (rerank by embedding cosine,
  no extra model), `website/repl/presets/between.lua` (blend two passages),
  alongside the existing `embed.lua` and `classify.lua`, all preloaded by
  `sandbox.yaml` and stamped by `just website-presets`.
- **Sandbox config**: `website/repl/sandbox.yaml` gains `quantize: auto`, the
  demo models with `max_length` sized for passages, and the new presets, with the
  on-volume model paths bumped so the first boot after the deploy downloads the
  quantized weights deterministically. `justfile`'s `download-model` gains an
  int8-aware sibling so local development matches.
- **Site bookkeeping**: `website/tools/published-tree.py`'s `SERVED` set,
  `website/.assetsignore`, `wrangler.jsonc` (asset size limits), `flake.nix`
  (`websiteDeps` gains the `sqlite-vec` build tool and its pin, which must stay
  substitutable).
- **Docs**: `website/README.md` (the gallery, the corpus pipeline, the model
  set), `README.md` (the demo model set and the quantization posture),
  `PRODUCT.md` (the gallery as demonstration, not offering), `DESIGN.md` (the
  atlas's composition).
- **Not changed**: the sandbox bridge, the allowlist, the reply contract, and
  `terminal.js`'s renderer. Image embedding stays refused; see the design's
  non-goals.
