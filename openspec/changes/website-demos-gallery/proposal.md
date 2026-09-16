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

- **A demos gallery** at `/demos`: an index plus ten demonstration plates
  (eleven when the dimension dial's measurement allows), built from the poster's
  existing atoms. Four tiers — what a vector is (the vector, the similarity),
  why vectors help (the search, the atlas), why `emb` and what a call costs (the
  model lens, the dimension dial, the model is a function, the image, the batch,
  the cache), and what a script computes beside the model (the graph).
- **The gallery is visual first.** Every plate carries a figure generated from
  live data, because 384 numbers in a paragraph explain less than 384 bars: the
  vector is drawn as a waveform of its values, the similarity as a distance on a
  scale, the search as ranked bars beside its quotations, the atlas as a map,
  the model lens as a morph, the image as label-probability bars, and the graph
  as a directed graph. Prose becomes the caption of the figure, not a
  replacement for it.
- **Visual embeddings are enabled, within limits.** A fused CLIP export joins
  the model set, so one space holds an image and a sentence and a picture can be
  scored against labels it was never trained on. Enabling that safely is the
  work: the plate ships **six public-domain samples** and accepts **no upload**,
  so the sandbox never receives a stranger's file; the bridge is told which
  argument is binary and base64-decodes it back to bytes only for the sandbox's
  own preloaded image preset, never for a raw command; the model is capped at two
  images per request, 256 KiB each, and one megapixel before decode; the bytes
  answer one request and are never stored. `EMB.IMG` and `EMB.IMGMULTI` stay
  outside the surface — only the sandbox's script sees an image.
- **Every figure animates, and the motion is the reader's leave.** One tween
  primitive (`tween(ms, step)`) draws the vector's waveform rising from the
  centre line, the graph's edges extending to their end, the image's probability
  bars growing, and the atlas's query landing and its neighbours flaring. With
  `prefers-reduced-motion` every figure is drawn complete in one frame and the
  end state is identical, so no result depends on an animation having run; the
  model lens's morph is the one place motion is the argument, and it becomes a
  cut.
- **The scripting layer computes structure, not just a value.** A `graph.lua`
  preset embeds its whole batch in one call, builds an N-by-N cosine matrix
  beside the model, and returns each node's nearest neighbours as edges — so
  sixty-four cosines become sixteen edges inside the server and the matrix never
  crosses the wire. The plate draws the result as a directed graph, which is
  what the atlas's regions look like when you connect the dots.
- **The gallery shows what a call costs, from the server's own numbers.** Two
  plates carry the performance argument the rest of the site makes in prose. The
  **batch** measures six single calls against one call carrying six texts and
  draws the server's own `elapsed_us` to scale; the **cache** asks for one
  passage twice and reads the model's `cache_hits`/`cache_misses` around the
  pair, so the second answer is shown to leave the cache rather than asserted to.
  Neither plate presents a speed-up it did not measure, and neither can be made
  faster by the other's cache: the batch salts both sides of its comparison, and
  the cache reports whether its own first call was a hit or a miss.
- **The atlas's order switch is made honest.** The plate's switch between meaning
  and chronology now draws the named regions only in the meaning order, and
  places the query under the year order at the mean publication year of the
  neighbours it retrieved — so flipping the axis cannot stack ten region rings
  at the origin or send the query's mark to a non-finite coordinate. Switching
  the order redraws from data already held instead of re-querying the sandbox,
  and every plate states a failed interaction rather than rendering it as an
  empty result.
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
  already exposes, plus the one base64 image path the image preset needs. No new
  endpoint and no server-side vector store.
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
  drawn from the index, and a generated visualization per plate), its corpus
  pipeline (corpus → `emb` → committed `sqlite-vec` database), its browser-side
  retrieval, its use of the scripting surface through preloaded digests, its
  honest offline behaviour, and the model set the demos require.
- `visual-embeddings`: the image capability — the downscale-then-base64
  transport, the bridge decoding it only for the sandbox's image preset, the
  per-request image bounds (count, bytes, pixels), the refusal of raw image
  commands and of binary for any other call, the no-storage posture, and the
  scripted zero-shot call that uses the model's image branch.

### Modified Capabilities

- `sandbox-service`: the fixed surface no longer refuses the sandbox's own image
  preset; that one preloaded digest may receive a bounded binary argument, while
  `EMB.IMG`/`EMB.IMGMULTI` and binary on every other command stay refused, and
  the spend bounds gain an image count and byte cap.
- `product-site`: the site gains a third surface and a door to it; the gallery's
  composition is held to the poster's existing atoms and the committed type,
  contrast, and motion floors, exactly as the landing and documentation surfaces
  are.
- `site-deployment`: the published tree's served set is no longer one landing
  page and one documentation surface — it includes the gallery pages, their
  vendored wasm, and their database, and the check that asserts the tree learns
  about each.

## Impact

- **New surface**: `website/demos/` (index and ten plates now, eleven with the
  gated dimension dial, including `image.html`, `graph.html`, `batch.html`, and
  `cache.html`), `website/assets/js/` (a gallery client module),
  `website/assets/vendor/` (pinned `sqlite-wasm-vec`).
- **New build tooling**: `website/tools/build-demo-db.py`, a corpus-acquisition
  tool (`website/tools/demos/fetch-poe.py`), the committed corpus
  (`website/tools/demos/poe.jsonl`), and a `just website-demos` target.
- **New presets**: `website/repl/presets/rank.lua` (rerank by embedding cosine,
  no extra model), `website/repl/presets/between.lua` (blend two passages),
  `website/repl/presets/graph.lua` (an N-by-N nearest-neighbour graph reduced to
  edges), and `website/repl/presets/zeroshot.lua` (the image branch), alongside
  the existing `embed.lua` and `classify.lua`, all preloaded by `sandbox.yaml`
  and stamped by `just website-presets`.
- **Sandbox and bridge**: `website/repl/sandbox.yaml` gains `quantize: auto`, the
  demo models with `max_length` sized for passages, the fused `clip` model, the
  new presets, and the image bounds (`max_images`, `max_image_bytes`,
  `max_image_pixels`), with the on-volume model paths bumped so the first boot
  after the deploy downloads the quantized weights deterministically.
  `website/repl/bridge.go` and `limits.go` gain the base64 argument form, its
  image count and byte caps, and the refusal of binary for any call but the
  image preset. `justfile`'s `download-model` gains an int8-aware sibling so
  local development matches.
- **Site bookkeeping**: `website/tools/published-tree.py`'s `SERVED` set,
  `website/.assetsignore`, `wrangler.jsonc` (asset size limits), `flake.nix`
  (`websiteDeps` gains the `sqlite-vec` build tool and its pin, which must stay
  substitutable).
- **Docs**: `website/README.md` (the gallery, the corpus pipeline, the model
  set), `README.md` (the demo model set and the quantization posture),
  `PRODUCT.md` (the gallery as demonstration, not offering), `DESIGN.md` (the
  atlas's composition).
- **Not changed**: the reply contract and `terminal.js`'s renderer. The bridge
  changes only to carry a bounded binary argument for the sandbox's own image
  preset; raw image commands stay refused, and no page may invent a host, a
  script, or a model.
