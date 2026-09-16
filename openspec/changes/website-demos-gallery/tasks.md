## 1. Quantize the sandbox model set

- [x] 1.1 Add `quantize: auto` to every model in `website/repl/sandbox.yaml` and bump each model's on-volume `onnx` path to `/data/models/int8/<name>/model.onnx`, so the first boot after the deploy downloads the quantized weights instead of early-returning on the existing fp32 file; verify with a local run (`just sandbox-run config=website/repl/sandbox.yaml`) that the load log reports `quantization=int8` and that `EMB.INFO minilm` reports `quantization: int8`
- [x] 1.2 Add an int8-aware local download target (`just download-model-quantized`) that fetches `onnx/model_quantized.onnx` to `model_quantized.onnx` beside the fp32 file; verify a local `emb` loads the quantized file and reports `quantization=int8`, so local runs match the sandbox
- [x] 1.3 Deploy the sandbox and verify `/api/ready` turns true inside the 60 s health-check grace on the first boot, then record steady RSS from `EMB.STATS` in the change's notes; if the boot exceeds the grace, stop and reassess rather than raising it

## 2. The demo model set

- [x] 2.1 Add `bge-small-en-v1.5` (384-d, retrieval) and `paraphrase-multilingual-MiniLM-L12-v2` (384-d, 50+ languages) to `website/repl/sandbox.yaml` with `model_repo`, `quantize: auto`, `normalize: true`, and a `max_length` sized for a passage (512 for the passage-level models); verify both load, `EMB.MODELS` lists every model with its dimension and status, and each returns a 384-element vector for a probe passage
- [x] 2.2 Deploy and verify the machine's steady RSS from `EMB.STATS` leaves headroom, and that one `EMB.MULTI minilm <t> bge <t>` through the bridge returns a reply per pair; record the RSS figure
- [x] 2.3 **Gated on 2.2 — decision: skip `all-mpnet-base-v2` (768-d), and with it the dimension dial.** Deployed RSS is 1 097 MB of 2 GB with `clip` loaded, so `mpnet` (110.1 MB int8) would probably fit, but the headroom is kept for the boot grace and the dial's lesson is already the lens's; the machine stays at 2 GB and the measured number is recorded in the change's notes rather than resizing on a guess
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
- [x] 7.8 **Gated on 2.3 — not built:** `mpnet` is not added, so the dimension-dial plate is skipped with it and the gallery ships ten plates; the decision and the measured headroom are recorded in the change's notes
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
- [x] 10.5 Run `openspec validate website-demos-gallery --strict` and verify the change validates, then deploy and verify the origin serves the gallery's pages, wasm, and database and that a rebuild is not served stale

## 11. Correct the atlas's order switch

- [x] 11.1 Draw the named region rings and their labels only in the meaning order, and verify by placing the atlas by year that no region ring or label is drawn and none is left with a zero radius or an off-plate coordinate
- [x] 11.2 Carry the mean year of the retrieved neighbours on the query's centroid and place it under the year order, and verify the query mark's `cx` is finite and sits at the mean year of what it retrieved
- [x] 11.3 Redraw the query and the marks from data already held when the order switches, without issuing another sandbox command, and verify the switch makes no request and leaves the readout's layout line correct
- [x] 11.4 State an empty result and a failed blend instead of rendering them as empty, and verify with the sandbox blocked that the blend states the condition, offers a retry, and shows no passage and no fabricated value
- [x] 11.5 Read only `rowid, year` for the year order rather than every passage's text, and verify the atlas places correctly and the year order no longer materializes the whole corpus in the page

## 12. The cost plates

- [x] 12.1 Add `website/demos/batch.html`: six single `EMB` calls against one `EMB` carrying six texts, drawn to scale from the server's own `elapsed_us`, and verify a live run draws both bars and reports the measured ratio
- [x] 12.2 Salt both sides of the batch comparison with a fresh per-run marker so neither side reads the other's cache, and verify the batched call is not served from the six calls' entries (the first draft measured 297× and was wrong)
- [x] 12.3 Add `website/demos/cache.html`: one passage embedded twice with an `EMB.INFO` on each side, reporting the cache counter deltas and stating whether the first call was a hit or a miss; verify a cold passage reports `+1 miss · +1 hit` and shows the hit's lower time, and a warm passage reports both as hits rather than a fake miss
- [x] 12.4 Link both plates from `website/demos/index.html`, state what each teaches, and update the index's plate count and tier copy from six to eight
- [x] 12.5 Add both pages to `published-tree.py`'s `SERVED` and `PAGES`, and verify `python3 website/tools/published-tree.py` passes and reports every internal reference resolving
- [x] 12.6 Verify both plates carry the five sections in order, one accent, passages (where used) attributed, and that the two plates add no custom property, colour, font, texture, or stylesheet rule — they reuse the atlas's SVG atoms for their bars

## 13. Verify the corrections

- [x] 13.1 Run `just website-published`, `just website-presets-check`, `just website-ink` over the gallery, landing, and docs surfaces, and verify all pass with no ink outside the viewport at any tested width
- [x] 13.2 Against a local `emb`, verify in a browser that the atlas places correctly in both orders with a finite query mark and no region rings by year, that the batch reports a measured several-fold gain, and that the cache reports a cold miss then a hit with the server's counters
- [x] 13.3 Run `openspec validate website-demos-gallery --strict` and verify the change validates with the new requirements

## 14. Visual first

- [x] 14.1 Draw the vector's 384 values as a waveform on `vector.html` — one bar per value above and below a centre line, scaled to the largest magnitude, with the single largest value the only accent — and verify a live run draws 384 bars and 1 accent from the reply's own floats
- [x] 14.2 Trim the prose on the vector plate so each section reads beside the figure, and verify no section restates what the waveform shows
- [x] 14.3 Build the batch, cache, image, and graph figures from the atlas's existing SVG atoms (`.atlas__svg`, `.atlas__mark`, `.atlas__mark.is-hit`, `.atlas__region`, `.atlas__label`) and verify the four new figures add no custom property, colour, font, or texture beyond the two small gallery rules declared
- [x] 14.4 Verify with `just website-ink` that every changed plate still passes at all 24 widths, including the 1086px reference frame and 390px

## 15. Visual embeddings, enabled with limits

- [x] 15.1 Add the fused `clip` model (int8, 512-d, image block) and the `zeroshot.lua` preset to `website/repl/sandbox.yaml`; verify locally that the model loads, that `emb.image.preprocess` and the image branch run, and that the preset scores labels for an image
- [x] 15.2 Write `zeroshot.lua` to supply the fused graph's unused branch as a host-built constant (`fill = 0`), request one output per run, reduce packed float32 with `emb.math`, and refuse more than eight labels; verify against a local `emb` that an image returns a label distribution
- [x] 15.3 Add the base64 binary transport to the bridge (`execRequest.Bin`, `decodeBinary`, `isImageCall`) and admit it only for the sandbox's `zeroshot` digest, with tests for the text-preset refusal, the raw-command refusal, the count cap, the byte cap, malformed base64, and an out-of-range index
- [x] 15.4 Add `MaxImages` and `MaxImageBytes` to the bridge's limits and the matching `max_images`, `max_image_bytes`, and `max_image_pixels` to `sandbox.yaml`; verify the bridge and the server both refuse an oversized image, and that a decoded image is exempt from the text-byte cap and not from the image cap
- [x] 15.5 Add `image.html`: a drop target and a drawn-pattern fallback, a browser downscale to a 512px long edge, base64 through `imagePreset`, and label-probability bars with the top label the single accent; verify a live run downscales, sends under the cap, and renders a distribution from the server
- [x] 15.6 Verify `EMB.IMG` and `EMB.IMGMULTI` are still refused through the bridge, that binary on any other command is refused, and that no image byte is stored or returned to another visitor
- [x] 15.7 Deploy and measure the `clip` model's steady RSS from `EMB.STATS`; if it does not fit beside the other int8 models at 2 GB, record the figure and ship the gallery without the image plate rather than resizing on a guess

## 16. The graph, computed by a script

- [x] 16.1 Write `graph.lua` — one batched `emb.embed`, an N-by-N cosine matrix reduced host-side by `emb.math.topk`, one value per key with its outgoing edges and its similarity to the query — and list it under `minilm` in `sandbox.yaml`
- [x] 16.2 Add `graph.html`: seed the corpus with a query, fetch eight passages, call the preset by digest, and draw the edges as a directed graph with the strongest edge the single accent; verify a live run draws 8 nodes and 16 edges and names the strongest edge
- [x] 16.3 Verify the page reads each node's nested edge arrays back into `{to, score}` rather than reading the wire's flat pairs, and that the strongest-edge readout is a real score
- [x] 16.4 Link `image.html` and `graph.html` from the index, renumber the reading order to ten plates, and add both to `published-tree.py`'s `SERVED` and `PAGES`; verify `published-tree.py` passes
- [x] 16.5 Add `zeroshot` and `graph` to `stamp-presets.py` and verify `just website-presets --check` reports every digest current and preloaded

## 17. Verify the expansion

- [x] 17.1 Run `just website-published`, `just website-presets-check`, and `just website-ink` over the landing, docs, 404, and every gallery plate, and verify all pass
- [x] 17.2 Verify in a browser against a local `emb` that the vector waveform, the image labels, and the directed graph all render from live replies and degrade honestly with the sandbox blocked
- [x] 17.3 Run `go test ./website/repl/ -count=1` and verify the new bridge transport and its limits pass without widening the surface
- [x] 17.4 Run `openspec validate website-demos-gallery --strict` and verify the change validates with the image and visualization requirements

## 18. The model lens, samples, and motion

- [x] 18.1 Fix `lens.html`'s projection: the fitted bounds were nested under `bounds` while `at()` read `x0/x1/y0/y1`, so every coordinate was `NaN` and the plate rendered nothing; verify all 2 782 marks render and the two models' rankings differ
- [x] 18.2 Light the query's retrieved neighbours on the current projection and re-render after a search, and verify the highlighted marks appear on both lenses
- [x] 18.3 Replace the lens's CSS-transition morph with the shared `tween` primitive, and verify against motion allowed that the marks move between layouts and that under reduced motion the switch is a cut to the new layout rather than a stale map
- [x] 18.4 Add `tween(ms, step)` to `demos.js` — one primitive that runs `step(1)` once when the reader prefers reduced motion — and use it for the vector waveform, the graph's edges and nodes, the image's and the cost plates' bars, and the atlas's query ripple and neighbour flare; verify each figure is complete with reduced motion on and animates with it off
- [x] 18.5 Replace the image plate's upload with six public-domain samples (committed at a 512px long edge, with a credits note), keep the client-side bound, and verify a live run labels the raven `raven` and the seascape `storm at sea` with no file input present
- [x] 18.6 Add the samples and their credits note to `published-tree.py`/`.assetsignore`, and verify `published-tree.py` passes and the credits note is not served
- [x] 18.7 Run `just website-published`, `just website-presets-check`, and `just website-ink` over every changed plate, and verify all pass at all 24 widths; run `openspec validate website-demos-gallery --strict`

## 19. The cost figures are execution time, not network time

- [x] 19.1 Remove the client wall clock from `batch.html` — the page no longer times its own requests — and verify the plate draws and reports only the server's `elapsed_us`
- [x] 19.2 State on both cost plates that the figure is the bridge's bracket around the upstream command, with the network excluded, and verify the copy and the readout agree
- [x] 19.3 Tighten the `embedding-demos` cost requirement: the time a cost demo shows MUST be measured at or beside the server and MUST NOT be a client clock around the request; verify `openspec validate --strict` passes and no plate prints a wall-clock duration
