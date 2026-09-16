## 1. Quantize the sandbox model set

- [x] 1.1 Add `quantize: auto` to every model in `website/repl/sandbox.yaml` and bump each model's on-volume `onnx` path to `/data/models/int8/<name>/model.onnx`, so the first boot after the deploy downloads the quantized weights instead of early-returning on the existing fp32 file; verify with a local run (`just sandbox-run config=website/repl/sandbox.yaml`) that the load log reports `quantization=int8` and that `EMB.INFO minilm` reports `quantization: int8`
- [x] 1.2 Add an int8-aware local download target (`just download-model-quantized`) that fetches `onnx/model_quantized.onnx` to `model_quantized.onnx` beside the fp32 file; verify a local `emb` loads the quantized file and reports `quantization=int8`, so local runs match the sandbox
- [ ] 1.3 Deploy the sandbox and verify `/api/ready` turns true inside the 60 s health-check grace on the first boot, then record steady RSS from `EMB.STATS` in the change's notes; if the boot exceeds the grace, stop and reassess rather than raising it

## 2. The demo model set

- [x] 2.1 Add `bge-small-en-v1.5` (384-d, retrieval) and `paraphrase-multilingual-MiniLM-L12-v2` (384-d, 50+ languages) to `website/repl/sandbox.yaml` with `model_repo`, `quantize: auto`, `normalize: true`, and a `max_length` sized for a passage (512 for the passage-level models); verify both load, `EMB.MODELS` lists every model with its dimension and status, and each returns a 384-element vector for a probe passage
- [ ] 2.2 Deploy and verify the machine's steady RSS from `EMB.STATS` leaves headroom, and that one `EMB.MULTI minilm <t> bge <t>` through the bridge returns a reply per pair; record the RSS figure
- [ ] 2.3 **Gated on 2.2.** Add `all-mpnet-base-v2` (768-d) if the measured headroom allows, and verify `EMB.MODELS` reports dimension 768; if it does not fit at 2 GB, record the decision and the measured number in the change's notes and skip the dimension dial rather than resizing on a guess
- [x] 2.4 Verify the allowlist needs no change: `EMB <new-model> <text>`, `EMB.MULTI` across the new models, and `EMB.INFO <new-model>` all return replies through the bridge, and `EMB.IMG` is still refused

## 3. The Lua presets

- [x] 3.1 Author `website/repl/presets/rank.lua` — a query plus up to seven candidate passages, tokenized and embedded with `emb.embed`, ordered by `emb.similarity`, returning a ranked array of `{rank, doc, score}` — and verify against a local server that `EMB.EVSHA minilm <sha> 8 <query> <7 candidates>` returns them in descending score order
- [x] 3.2 Author `website/repl/presets/between.lua` — two texts blended with `emb.math.add`/`scale`, compared against the candidates, returning the nearest — and verify it returns a plausible midpoint item; if it reads as a gimmick in review, record the decision and drop it
- [x] 3.3 List all four presets in `website/repl/sandbox.yaml` under the models that run them, and verify a local `emb` preloads each and `EMB.EVSHA` accepts each digest while `EMB.EVAL` is still refused
- [x] 3.4 Run `just website-presets` and `just website-presets-check` and verify every demo digest is stamped from the shipped bytes, so the page cannot name a digest the sandbox was not told to load
- [x] 3.5 Confirm the bridge limits are respected rather than raised: verify a rerank is at most seven candidates because the bridge caps a request at `max_texts: 8`, and record that on the page

## 4. The corpus

- [x] 4.1 Write `website/tools/demos/fetch-poe.py` to acquire the public-domain tales and poems from Project Gutenberg plain-text editions, strip the acquisition boilerplate by its markers, and record each work's title and year; verify the tool asserts that no boilerplate marker survives in its output
- [x] 4.2 Chunk the corpus into passages — split at sentence boundaries to at most ~160 words, drop fragments under ~8 words, carry work, year, index, and a display snippet — and commit `website/tools/demos/poe.jsonl`; verify ids are unique, every passage is inside the bridge's per-text cap, and the passage count is recorded
- [x] 4.3 Record the corpus's licence and attribution beside it and verify the acquisition source, the public-domain status, and the attribution line the plate will display are all present in the repository
- [x] 4.4 Spot-check the chunking on a handful of known passages and verify the retrieval spot-check corpus reads sensibly (no mid-sentence headless fragments, no boilerplate, no swallowed dialogue breaks), recording the sample in the change's notes

## 5. The corpus index

- [x] 5.1 Write `website/tools/build-demo-db.py`: embed every passage against a local `emb` over RESP with batching, compute a 2D PCA projection at build time, run a small k-means over the projection, and write a `.db` with a `vec0` table per model, a passage-metadata table, the projection, the cluster regions, and a manifest (model, dimension, count, precision, source hash, truncated fraction, recall probe, cluster labels); verify a run produces both files and that naming a model that does not match the index fails loudly
- [x] 5.2 Hand-name the computed cluster regions (`THE SEA`, `THE GRAVE`, `MADNESS`, `DETECTION`, `GRIEF`, …) into the manifest and verify each named region's members read as a theme rather than a hash collision
- [x] 5.3 Verify the built index answers: run a `vec0` top-k for several thematic queries ("a guilty conscience that will not stay buried", "a ship in a storm", "the dead returning") and confirm the nearest passages are from the expected works, recording the rankings in the change's notes
- [x] 5.4 Build one `.db` carrying two `vec0` tables (`minilm`, `bge-small`) over the same corpus for the model lens, and verify a query against each table returns a **different** top-k — the incomparability the lens demonstrates
- [x] 5.5 Add `just website-poe` and `just website-demos` and verify a rebuild that changes the corpus produces a new content-hashed filename and manifest

## 6. The gallery client

- [x] 6.1 Vendor `sqlite-wasm-vec` pinned to an exact version, with a `just` target that re-vendors it the way `just website-player` does for the asciinema player; verify the vendored file exists, the pin is recorded in `flake.nix`'s comment, and no CDN URL ships
- [x] 6.2 Add `website/assets/js/demos.js` exposing the gallery's two calls — text in, ranked neighbours out (embedding via the existing `VALUES` contract, then a `vec0` top-k); and a preset by digest, envelope out — and verify against a local sandbox that both return real data and that the module captures the sandbox origin from its own `src` rather than a hardcoded host
- [x] 6.3 Lazy-load the wasm and the database on first interaction with a plate, and verify the gallery index fetches neither until a plate is used
- [x] 6.4 Implement the unavailable, starting, and capacity states in the console's existing vocabulary, and verify with the sandbox blocked that no vector, reply, or ranking is shown, a retry is offered, and the page's static content remains readable
- [x] 6.5 Honour `prefers-reduced-motion`, and verify no plate animates without being asked and no result depends on an animation having run

## 7. The gallery plates

- [x] 7.1 Add the gallery index at `website/demos/index.html` listing every plate with the one thing it teaches and the reading order, built from the poster's rule-head and ruled numbered entries; verify every plate is linked, the first-timer's starting point is named, and the corpus attribution is present
- [x] 7.2 Plate I — **the vector**: embed a typed passage and render the dimension, the L2 norm, and the bytes, with the mechanism section stating the tokenize → encode → pool → normalize path; verify the figures come from the server reply, not the page
- [x] 7.3 Plate I — **the similarity**: two passages through the `embed` preset returning `{dim, norm, similarity}`, with the Lua source and its digest beside the reply; verify the reply is the server's and the digest matches the stamped value
- [x] 7.4 Plate II — **the search**: a thematic query embedded live, the corpus searched with `vec0` top-k, the result reranked through `rank.lua`, results shown as quotations with work and year, and the exact `EMB …`, the `SELECT … WHERE embedding MATCH ? ORDER BY distance LIMIT k`, and the `EMB.EVSHA … rank` shown together; verify a query phrased in words the corpus does not contain returns the expected passages
- [x] 7.5 Plate II — **the atlas**: the build-time projection as engraved marks on the dark plate, the cluster regions labelled from the manifest, the live query landing and its neighbours lighting, and an order switch that places passages by meaning or by year; verify the query point lands near its ranked neighbours, no runtime projection work happens, and the caption carries the index's real figures
- [x] 7.6 Plate III — **the model lens**: one corpus, two `vec0` tables, the same query against each, and the projection morphing between the two models; verify the two models' top-k differ for a probe query and that the incomparability statement is present
- [x] 7.7 Plate III — **the model is a function**: the same passage through raw `EMB`, the `embed` preset, and the `classify` preset, showing the four reply shapes side by side with each script's source and digest; verify all four replies are the server's and that the page states raw Lua is refused
- [ ] 7.8 **Gated on 2.3.** Plate III — **the dimension dial**: `minilm` (384) against `mpnet` (768) over one corpus, stating that dimension is a cost/quality dial; verify both rankings render and each model's dimension is read from the server
- [x] 7.9 Give every plate the same five sections in the same order — what you are looking at, try it, what just happened, why it matters, the exact commands — and verify by inspection that all five are present in order on every plate and that each command shown is the command that was issued
- [x] 7.10 Apply the instrument rules to every plate: one accent spent on meaning, ruled figure captions carrying the index's real figures, passages as attributed quotations, paper for explanation and the dark plate for instrumentation; verify by inspection that no plate introduces a colour, font, texture, card, gradient, rounded panel, or shadow the stylesheet does not already declare
- [x] 7.11 Review the optional raven mark against the plates; verify that with the mark cut every plate still reads as complete, and record the include-or-cut decision in the change's notes
- [x] 7.12 Verify every plate at the 1086px reference frame and at 390px: no text computes below the type floor, every control clears the target floor, every text and control colour clears its ratio, and the stylesheet declares no new custom property

## 8. Bookkeeping

- [x] 8.1 Add the gallery pages, `assets/js/demos.js`, the vendored wasm, and the content-hashed `.db` to `published-tree.py`'s `SERVED`, add the gallery pages to `PAGES` so each declares an absolute canonical and social image, and add the corpus and tools to `.assetsignore`; verify `python3 website/tools/published-tree.py` passes and reports the corpus and tools as absent
- [x] 8.2 Add the `sqlite-vec` build dependency to `flake.nix`'s `websiteDeps`, and verify it stays substitutable with the documented check (`nix-store -qR "$(nix eval --raw .#devShells.aarch64-darwin.website.drvPath)" | grep '\.source.*\.drv$'`); if nixpkgs does not carry it, pin and vendor it the way the player is
- [x] 8.3 Add `website/demos/` and `website/assets/demo/` to `ci.yml`'s site path filter, and verify a change touching a gallery page runs the site's checks while leaving the server and gem jobs skipped
- [x] 8.4 Give every generated index a content hash in its filename and confirm `_headers` never pins an unhashed generated asset immutable; verify that rebuilding an index changes its name and that a returning visitor cannot receive the previous index

## 9. Documentation

- [x] 9.1 Update `website/README.md` with the gallery, the corpus pipeline (`just website-poe`, `just website-demos`), the presets, the vendored wasm, and what ships versus what is excluded, and verify the published-tree section agrees with `published-tree.py`
- [x] 9.2 Update `README.md` with the sandbox's demo model set, the int8 posture, and the `quantize` settings the deployed config uses, and verify every model named is one the server loads
- [x] 9.3 Update `PRODUCT.md` to describe the gallery as a demonstration with a fixed public-domain corpus and ephemeral visitor input — no account, no stored text, no hosted implication — and verify no commercial fact is introduced
- [x] 9.4 Record the atlas's composition in `DESIGN.md` — the instrument rules, the one-signal rule, the plate captions, the quotation treatment, and the motion contract — and verify the stylesheet declares no token the document does not name

## 10. Verification

- [x] 10.1 Run `just website-published`, `just website-presets`, `just website-ink`, and `just website-shot` over the gallery, the landing and the documentation surface, and verify all pass with no ink outside the viewport at any tested width
- [x] 10.2 Run `impeccable detect --json` over the changed HTML, CSS, and JS and resolve or record every finding
- [x] 10.3 Verify the sandbox is otherwise untouched: `just sandbox-test` passes, `website/repl/bridge.go`, `allowlist.go`, and `terminal.js` are unchanged, and the refuse list still includes raw scripts, images, config, and writes
- [x] 10.4 Verify the gallery with scripting disabled — every plate carries its explanation, mechanism, scripts and commands and states that the interactive part requires scripting — and verify a plate with the sandbox blocked shows its unavailable state and no fabricated result
- [ ] 10.5 Run `openspec validate website-demos-gallery --strict` and verify the change validates, then deploy and verify the origin serves the gallery's pages, wasm, and database and that a rebuild is not served stale
