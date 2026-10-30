# Proposal

## Why

emb serves DINOv3's dense features — one vector per 16-pixel patch — and the plate that draws them reads as an essay with a figure in the middle: four prose blocks, two definition lists, a code specimen, and the canvas two screens down. The brief for this plate is the opposite of an essay. It is one photograph that becomes a world and then becomes mathematics, and a visitor has to see that before reading a word of it.

So the plate does what the published DINOv3 demos do not: it rebuilds one photograph as a lit surface from a real monocular depth model — **shaded by the depth map used as a normal map**, with shadows walked over that same height field — and it lets the reader use the features as the index they are: **point at a part and find it in the other five photographs**, **let the model group the picture into its own parts**, and **crop a region and ask where it came from**, with the page grading the answer because it drew the box. The first screen is the figure: a stage filling most of the viewport, the photograph, and one action that starts the reveal. The arithmetic, the licences and the commands are one disclosure away, because the page has to be spectacular first and checkable second.

Three facts decide the shape of the work:

- **Depth is not a DINOv3 feature.** A monocular depth map requires a trained decoder (`docs/image-models.md`). The sandbox must run a second image model — Depth Anything V2 Small (`onnx-community/depth-anything-v2-small`, Apache-2.0, int8 27 MB) — and the server must be able to return a **2-D map**, not just rows of tokens.
- **The vertices are the photograph.** One vertex per pixel of the bounded image keeps the colour and detail recognisable; the depth map only displaces them. The reconstruction's aspect, framing and colours are the photograph's, never a normalised square.
- **A search needs a library that contains matches.** Six photographs chosen by depth alone share no subject, and their feature cosines say so: the strip's ranking is mostly the model reporting that nothing here matches. The photographs are therefore chosen on two measured axes, and the second one — whether their features can find each other — is the one that makes the demo's claim true.

## What Changes

> The bullets below record the two-model build as it was delivered and measured. **“The depth plate”** supersedes the DINOv3 half of them: that model, and every figure built on it, is removed.

- **The plate is a stage, not an essay.** The plate's own head carries the title, two sentences and the ruled figure caption. Directly under it sits the stage — `clamp(360px, 78vh, 760px)` tall, full shell width, near-black — with the photograph already in it, revealing itself on arrival; the filmstrip sits directly under it. Nothing else is visible above the fold. Below the stage the copy follows the gallery's own sections — what you are looking at, what just happened, why it matters, the exact commands — because the plate explains mechanisms the way every other plate does; only the deepest arithmetic stays in one disclosure. Text budget: **two sentences above the figure**, and no prose section above it either.
- **The reveal runs itself, and can be interrupted.** Every asset reveals when its replies land, so `Reveal perception` is the replay rather than the way in. The sequence is 6.1 s on one clock: **dissolve** (900 ms, the raster photograph hands over to its own pixels as points), **depth awakening** (3 000 ms, the flat field back-projects into the depth model's map, staggered deterministically), **camera reveal** (2 200 ms, a 12.6° orbit that shows real parallax and stops) — and the depth map's gradient arrives as light with the camera, so the lift is first seen as the still's own displacement instead of a fade through black. Any pointer press, wheel, double-click or key input cancels it; `prefers-reduced-motion` lands on the settled relief in one frame; a sample switch resets to that asset's photograph and reveals it, as does an upload.
- **The stage is the control surface, the rest sits behind one disclosure.** Drag, wheel, hover and click are the interaction. Depth exaggeration, the cut, the field density, `isolate`, the upload and the probe's column and row live in one `adjust` disclosure; the state and reading controls stay visible, and the inspector — the plan, the probe and the readout — stays under the stage, tracks the same patch across every state, and never asks the server for a second call.
- **The camera is clamped to the reconstruction's one face.** Yaw ±25°, pitch ±15°, zoom on a wheel with limits, double-click resets. The orbit shows foreground, middle ground and background as separate planes without ever opening the missing backside.
- **Two states are one field of vertices.**
  - **THE PHOTOGRAPH** — full-bleed and untouched, with one action that starts the reveal.
  - **THE RELIEF** — one vertex per pixel, colour sampled from the photograph, position the pinhole back-projection of that pixel at the depth the model predicts, and **normal from the depth map's own gradient**, lit by a fixed key light plus a fill. The exaggeration control scales the slope and the lift together, so the shading follows the relief. The still photograph is never lit, so it stays exactly the source.
- **The probe is the model's second answer, and it reaches across pictures.** Hovering or clicking takes the patch's returned vector, ranks it against every patch of this picture and of the other five (their grids are fetched once, lazily, and kept), and prints the best score and cell per picture in a strip. A result opens that photograph at the patch the model chose. No index, no vector database, no second model: a dot product over grids that came out of the same graph.
- **The reconstruction keeps the photograph's scale.** Positions are computed in image proportions (`x = (u − ½)·(W/H)`, `y = ½ − v`), so a 3:2 photograph stays 3:2 in every state and the depth planes do not stretch. The model's square patch grid is mapped back onto the photograph's own aspect, in the scene and on the plan view.
- **The figure is one indexed surface, cut by topology.** A single vertex buffer — position on the image plane, image UV, colour, the depth model's ordering, its gradient and a seed — feeds one `Mesh` whose index is generated after a depth test: a cell survives only when its four edges and its diagonal each change the display depth by less than the tolerance, so the wireframe cannot draw an edge the surface does not have and no triangle spans two objects. The mesh and the cloud of the earlier build were the same vertices at two densities, and the wireframe, the contours and the separate cloud are deleted rather than kept beside it: 57 088 · 128 640 · 513 792 vertices at the three density tiers for one 768-pixel bounded sample, painted from the photograph's own colours, one draw call, no per-frame allocation.
- **A second image model on the sandbox, and a map output on the server.** `EMB.IMG depth BLOB OUTPUTS depth <one image>` returns a relative depth map as packed float32. The server gains one output selector — `map`, for a rank-3 `[1,H,W]` (or rank-4 `[1,1,H,W]`) tensor — reported by `EMB.INFO` as `{kind: map, rows: H, dim: W}` and never normalized, because depth is not a unit vector. The depth model runs at 266 (19×14), so one map is 283 024 B packed, 377 KB base64 on the wire.
- **The bridge admits the sandbox's two image models, nothing wider.** `EMB.IMG <model> BLOB [OUTPUTS <name>[,<name>…]] <one image>` where `<model>` is `dinov3` (embedding, patches) or `depth` (depth); one image per call; `VALUES`, `EMB.IMGMULTI`, any other model, unknown output names and a second image stay refused with a reason.
- **Six curated photographs replace the artworks** for this plate, in `website/demos/samples/parts/`, chosen by measurement rather than by eye: every candidate was run through the plate's depth model and kept only where the normalised map spans the whole range — a near foreground, a middle ground and a far background, both deciles occupied, with fine gradient energy — so a flat frame, a night frame or one all in the far field never ships. The first pass chose by depth alone, and the feature cosines showed the cost of it (no two pictures share a subject); the re-curation adds a second gate — part structure and at least one same-subject pair — and keeps the depth one. In filmstrip order, with the first as the default: a curving autumn road (Aaron Burden, CC0), the Lincoln Cathedral nave (Michael D Beckwith, CC0), an autumn forest path (Vovogov90, CC0), a castle courtyard looking out (Michał Bulsa, CC0), the Yellowstone River in its canyon (Jacob W. Frank / NPS, public domain) and the Seven Arches aqueduct (Chemical Engineer, public domain). Four CC0 and two public domain from Wikimedia Commons, committed at a 768-pixel long edge with their native aspect and a `CREDITS.md` row each. Thumbnails letterbox (`object-fit: contain`) instead of cropping, so the filmstrip shows each photograph's own shape.
- **The relief scene is deleted**, not kept beside the new one: its square surface, its grid cells and its plan-view tinting are replaced by the depth-shaded field and by a plan canvas that still carries the probe's similarity mask at the photograph's resolution.
- **The plate never draws a stored figure.** The brief asks for committed depth and feature assets so the featured example renders instantly; the gallery's contract forbids drawing from stored data, so the plate pays one `dinov3` call and one `depth` call per sample and prints the measured times. Both are content-hash cached, so a repeat is a lookup.
- **The reconstruction is an indexed mesh with two readings, and the features get a third.** The depth half reads the same buffer twice — the surface, and camera-space normals from the depth map's own gradient — under one key light, a fill, a specular that follows the camera, and cast plus contact shadows walked over the same blurred height field. The feature half stops being a number: `parts` clusters the returned patch rows and paints each vertex with its cluster, so the picture breaks into the groups the model found without ever being told what they are. Every reading is a uniform over one buffer, so the cut, the grown region, the probe and the camera compose with all three.
- **The region is grown, and the depth becomes a tool.** A probe grows a spatial region over patch connectivity, bounded by the model's cosine, the photograph's colour and the depth map; the region's mean vector is the query. `isolate` keeps that region and drops the rest to a trace, with the region's approximate box. The `normals` view paints the depth gradient's orientation instead of the photograph, and three density tiers (3 · 2 · 1 pixels per step) re-sample the same photograph without a second call. An uploaded photograph takes the same two calls and stores nothing.
- **Accessibility and honest limits survive.** The plan canvas is the `role="img"` figure a screen reader gets and the whole figure without WebGL; reduced motion skips the reveal; a hover, a click and a keyboard row/column control share one probe; the copy states that the depth is monocular and approximate, that no invisible backside is invented, and that a high cosine in another picture is a similar patch, not the same object.
- **A crop is a question the page can grade.** Dragging a box on the photograph sends that box as its own image, and the plate asks the model where it came from: the crop's centre patch block is the query, the answer is a cell on the grid, and the page knows the right answer because it drew the box. The readout prints the rank and whether the answer fell inside the crop's own cells, so the reader can check the model instead of admiring it. The same query ranks the other photographs, which makes "which picture is this from?" a question with an answer rather than a claim. Measured on the shipped model: seven of nine crops land inside their own box, six of them at rank one.
- **The library is chosen so its rankings mean something.** The six photographs were curated by depth alone, and their feature cosines show it: the best cross-picture patch cosine is 0.32–0.67 and the pooled vectors sit at 0.13–0.43, so the strip's "0.51, 0.44, 0.37" is the model saying nothing here matches. The library is re-curated on two axes — the depth gate it already had, and a matchability gate that requires at least one same-subject pair and a measured median patch cosine — because a search demo whose own library contains no matches cannot demonstrate a search.
- **The grid stays at 196 unless a measurement says otherwise.** The sandbox serves the 224 export: 14 × 14 = 196 patches, and a cluster at that resolution is a fat block. A 336 export (21 × 21 = 441) is already supported and tested in the registry; switching it costs 2.25× the tokens and roughly five times the attention, so the change is gated on a measurement of the cold call on the deployed host rather than on the appeal of a finer picture.

## The depth plate (supersedes the two-model showcase)

**Decision.** The plate keeps one model. DINOv3 is removed from the plate, from the sandbox's model set and from the bridge's image surface; the plate becomes a **depth** demonstration, because the depth half is the one that survives "is this useful, and is it different?" and because patch selection was never legible to a reader who had not already met patch features.

**Why DINOv3 goes, in its own measurements.** The council (D33–D35) measured every candidate the page could still offer beyond a similarity display, and each one died on a number the page itself prints:

- every use is one cosine wearing a different coat: the matrix, the field, the probe, the crop and the strip are all `dot(row, row)` over the same 196 × 384 reply;
- the grouping is not better than a ruler: a content-blind 8-band cut scores **0.731** four-neighbour coherence against the model's **0.670–0.766**, i.e. better on one picture of six;
- the one scalar that is not a pairwise cosine (typicality against the pooled vector) is **88–97%** explained by the cluster a patch already belongs to;
- the strongest remaining claim, cross-photograph correspondence, is silent on **79–81%** of the second picture's cells, and its "same composition, different place" band (0.492) sits *inside* the unrelated range (0.371–0.556);
- the direction search that would have been a different *kind* of interaction never clears the picture's floor: **0%** of 400 random queries.

A demo whose own numbers say "this is a cosine, and the baseline matches it" is not a demo a reader can restate. The depth model's map is spatial, checkable by eye, and photographic, and it is the half a reader can *use*.

**What the plate becomes.** One photograph, one call (`EMB.IMG depth BLOB OUTPUTS depth`), one surface — and three ways to read the same 266 × 266 map, each drawn from the reply the page already holds:

```
one photograph, bounded to 768 px
        │  EMB.IMG depth BLOB OUTPUTS depth          <- one call, cached by content hash
        ▼
   266 × 266 relative depth, packed float32
        │
        ├── THE RELIEF      the surface, the cut, the exaggeration, the camera (unchanged)
        ├── THE MAP         the reply itself, beside the picture, pixel for pixel
        ├── THE PROFILE     drag a line: the depth along it, and the same numbers on the relief
        └── THE FOCUS       the map as a depth of field: refocus the photograph by depth
```

**1. The map is a figure, and it is the picture's own size.** The reply is drawn as a 266² raster under a `raw` / `rank-equalised` toggle, beside the photograph at the same aspect, with a hover readout of the exact float the server sent and a legend built from the map's own p5/p95. It replaces the plan canvas's old job (a tint of the probe's similarity), keeps `role="img"`, and its label states the shape, the range and the value under the pointer.

**2. The profile turns the relief into a measurand.** Drag a line across the photograph (or set start/end with two native range inputs inside `adjust`); the page samples the map along it with the same bilinear reader the geometry uses and draws the profile in display units — the exact numbers the vertex shader displaces by — with the two ends marked on the photograph and on the relief. The readout prints the near and far values, the mean absolute slope, and the span in frame-heights. The claim is honest by construction: it is the shader's own height function, not a second interpretation.

**3. The focus is the depth map used as a photographic tool.** Two native inputs — a focus distance (as a percentile of the map, never metres) and an aperture (the width of the sharp band) — blur the photograph by `|d̂ − focus|` in six quantised bands drawn with `ctx.filter`, masked by the map itself. A sharpness readout (Laplacian variance on a 192-pixel copy) proves the focused band is sharp and the rest is not, and the places the map is wrong (glass, a thin branch, the tunnel's opening) stay wrong on screen: the figure demonstrates a depth of field **and** the map's failures in the same gesture.

**4. The trace shrinks to one lane.** The run is one command now: the bounded JPEG's byte count, one head with its measured time, one reply bar scaled by `count × 4` with its shape from `EMB.INFO`. It keeps the rule that no number is typed twice, and it is DOM text, not SVG.

**What is deleted, by name.**

- *Page:* the `parts` reading and its k-means; the probe, the grown region and `isolate`; the crop box, its graded answer and the library search; the token strip; the grouping mosaic and its ruler; the correspondence field and its ladder; the six-picture answer strip (the filmstrip stays as a picker only); the `aPart`/`aPatch` attributes, `uParts`, `uSim`, `paintSim`, `growRegion`, `regionQuery`, `drawBox`, `buildPatchStats`, `clusterPatches`, `clusterCoherence`, `buildMosaic`, `buildStrip`, `buildField`, `selfFloor`, the DINOv3 half of `run`, and the second lane of the trace.
- *Sandbox:* the `dinov3` model entry (~90 MB resident, 22 MB on disk) and its image sessions; the model is not used by any other plate.
- *Bridge:* `dinov3` from `imageModels`, from the surface string and from the refusal text, leaving one image model; the allowlist tests and the bridge table lose the `dinov3` rows and keep a refusal that names `depth`.
- *Server:* nothing. The `select: tokens` capability stays — it is a registry feature with its own change and tests, and other clients may use it.
- *Copy:* every sentence about patches, vectors, cosines, clusters, crops or correspondence; the definitions list keeps only the depth limits.

**What stays exactly as it is.** The map selector and the depth model, the stage-first layout and its text budget, the reveal and its clock, the surface and normals readings, the cut and its measured refusal share, the exaggeration slider, the camera clamps, the keep-the-photograph's-aspect rules, the six depth-chosen Unsplash photographs and their credits, the plan canvas as the accessible and WebGL-free figure, and the four prose sections with their disclosures.

**Numbers the new figures must print, all from the reply.**

| figure | numbers | source |
|---|---|---|
| the map | `rows × dim`, min/median/max, p5–p95, the value under the pointer | `EMB.INFO` and the packed reply |
| the profile | near and far values, mean absolute slope, span in frame-heights, the two endpoints' cells | the same bilinear reader the vertex shader uses |
| the focus | the focus percentile, the band's width in percentiles, the sharpness ratio (focused band ÷ outside), the render time | the map plus a Laplacian variance on a 192-px copy |

**Gates before this ships.**

1. **Map gate:** the raster is drawn from the reply; a hover value equals `depth[row × dim + col]` exactly; the count equals `rows × dim` or the figure refuses and says why.
2. **Profile gate:** 100 samples along a random line match the shader's height function within 1e-6 (they share the function); dragging issues **no** call; the endpoints' readout matches the map for those pixels.
3. **Focus gate:** the sharpness ratio at the focus band is **≥ 3** on every shipped picture; a slider change renders in **≤ 40 ms** and issues no call; the blurred canvas is the photograph the browser sent, never a server result.
4. **Removal gate:** no `dinov3` string in the page, the bridge or the sandbox config; the bridge refuses `EMB.IMG dinov3 …` with a message naming `depth`; `EMB.MODELS` lists one image model; the sandbox's resident set drops by the DINOv3 sessions.
5. **Site gate:** `node --check`, `website-pages-check` after the descriptor re-stamp, `website-presets-check`, `website-published`, `website-ink` at 24 widths, `impeccable detect` no worse than the plate's **8** shared findings, and a browser pass at 1440×900 and 390×844 with an empty console and **0 commands** on any figure interaction.
6. **Deploy:** the sandbox change is manual — `just sandbox-deploy` is now part of shipping this, because the model set changes.

**Risks and what answers them.**

- **A photographic effect read as a lens.** The focus figure can look like a filter. Answered by printing the sharpness ratio and the focus percentile, by naming the figure "the map as a depth of field", and by showing the failures the map has.
- **The plate loses its second-model story.** True, and the gallery's teaching line changes with it: the plate is now about one model measured against the picture, not two models against each other.
- **Deleting working code.** The deletion is the point; the record keeps the measurements that justify it (D33–D35) so a future reader can disagree with evidence rather than memory.
- **One call instead of two means less to watch.** The first sample now waits on one graph; the reveal and the relief carry the first screen.

**Migration and rollback.** The order is: delete the DINOv3 surface (page, bridge, sandbox) and prove the refusals; build the map, the profile and the focus; rewrite the copy; re-measure the gates; re-stamp the descriptor; deploy the sandbox. Rollback is restoring the `dinov3` entry in `sandbox.yaml` and the bridge's two-name allowlist, and the page from the commit before — the depth half is untouched by any of it.

### The rework: Unsplash depth, feature figures, and the name

The first build settled the server, the reveal, the three readings, the crop and the library. The rework addresses what is still weakest, and it is all site-side except for one photograph set: **the photographs do not show the depth model at its best, DINOv3's features are explained by prose instead of by figures, and the copy still argues where it could demonstrate.** The two releases (the server's `map` selector and the bridge's image pair) do not change.

#### 1. The library is Unsplash, chosen on the depth it carries and measured on what its features match

Six photographs, all Unsplash, all arriving through the Wikimedia Commons import of Unsplash releases that carry **CC0** (the plate's existing licence path: the Commons page is the licence of record, the Unsplash page is the source of the photograph, and the tree ships the mirror — no hotlink, no stock-site licence text in the tree, no weights). 657 candidates were harvested and every one was measured through the depth model before six were kept:

> **Superseded for the shipped set:** the plate was re-curated a second time for *kinds* of depth; the installed six and their measurements are in `design.md` D27. The table below records the first Unsplash pass.

| # | File | Scene | Size | p5 | p95 | spread | near &lt;0.10 | far &gt;0.90 | gradient | coherence |
|---|---|---|---|---|---|---|---|---|---|---|
| 1 | `parts/church-aisle.jpg` | a wedding aisle between rows of chairs, one-point | 768×513 | 0.005 | 0.947 | 0.943 | 0.369 | 0.098 | 0.0158 | 0.662 |
| 2 | `parts/bridge-fog.jpg` | a suspension bridge disappearing into fog | 768×509 | 0.000 | 0.920 | 0.920 | 0.500 | 0.065 | 0.0371 | 0.712 |
| 3 | `parts/snow-tunnel.jpg` | a rock tunnel opening onto a snowy valley | 768×512 | 0.000 | 0.830 | 0.830 | 0.166 | 0.014 | 0.0099 | 0.703 |
| 4 | `parts/pier-fog.jpg` | a pier running into fog, one chair at its end | 768×512 | 0.000 | 0.883 | 0.883 | 0.582 | 0.040 | 0.0178 | 0.717 |
| 5 | `parts/canyon-rim.jpg` | the Grand Canyon's layered rim at dusk | 768×576 | 0.000 | 0.912 | 0.912 | 0.466 | 0.057 | 0.0114 | 0.761 |
| 6 | `parts/canyon-clouds.jpg` | the same canyon under a bank of cloud | 768×576 | 0.000 | 0.845 | 0.845 | 0.455 | 0.029 | 0.0078 | 0.692 |

- **Credits** — one row per file in `samples/CREDITS.md` with the work, the photographer, the Unsplash handle, the Unsplash page and the Commons mirror: church aisle (Shardayyy Photography, `shardayyy`), bridge in fog (Antoine Meillet, `antoinem`), snow tunnel (Thomas Kelley, `thkelley`), foggy pier (Camille Kimberly, `cam_bam`), canyon at dusk (Levi Jones, `levidjones1999`), canyon under cloud (Michael Hirsch, `hirsch`).
- **Why these six — four mechanisms, one place.** **Perspective convergence** (the aisle, the pier), **atmospheric falloff with a structure in it** (the bridge), **occlusion through a frame** (the tunnel) and **layered geology** (the canyon). Every one is a scene where a monocular map has an unambiguous ordering: the spread is 0.83–0.94, the near decile holds 17–58% of the frame with a real far decile behind it (1.4–9.8%), the gradient energy is 0.0078–0.0371, and each picture's eight clusters are contiguous at 0.66–0.76.
- **The pair is a place, not a cosine.** The two canyon frames are the same place shot by two different photographers (Levi Jones and Michael Hirsch), which is what makes them a same-place pair rather than an accidental match.
- **The ladder the library prints.** Every pair of the six was measured, and the field is asymmetric in a way worth printing:
  - **same place, two frames** (canyon under cloud → canyon at dusk): median per-patch best **0.798**, pooled **0.678**; the other direction measures **0.784**;
  - **same composition, different place** (church aisle ↔ foggy pier, both one-point converging scenes): median **0.492**;
  - **everything else**: median **0.371–0.556**.
  So on this checkpoint **only the same place separates**: a shared composition does not lift a pair above unrelated scenes. That is the honest finding, and it is what the copy says.
- **What that changes in the copy.** The plate no longer says "find the same photograph" and no longer claims that a shared motif scores high. It says what was measured: **the field is dense where the place is the same, and composition alone does not lift a score** — and the pair, the aisle/pier "same composition" number and the unrelated floor are all printed so the reader can check the claim against the same library the stage is drawing.

#### 2. The feature showcase becomes a trace and a grouping test, not a matrix

A council (four advisors, one cross-exam; the memo is in `design.md` D33–D35) was convened on the question "what is the best style of DINOv3 demo", with seven candidate styles measured against the shipped replies. Two of them were already built and one was about to be: the **similarity matrix**, the **correspondence field** and a **grouping mosaic**. What survived measurement is below; what died is in D35 with the number that killed it.

**The matrix is deleted.** It printed the same floor the field prints, it was the third drawing of one cosine, and it is exactly what "not just the similarity" describes. Its five plan rings, its play/pause, its 12 ms of dot products and its 6-second sweep go with it.

**The trace replaces the static schematic.** The old graph hard-coded `1 × 384 · 196 × 384 · 1 536 B · 301 056 B` as SVG text — the one figure on the page exempt from "a figure cannot lie about the reply" — and its 11-px labels scaled to 5.3 px on a phone. The new figure is the run's own trace, in DOM text at the page's 12-px mono:

```
what came back                       93 050 B · one bounded JPEG · two calls, in sequence
DINOv3 ViT-S/16 · one pass · 324 µs
  patches  ████████████████████████████████████████  196 × 384 · 301 056 B
  pooled   ▎                                          1 × 384 · 1 536 B
Depth Anything V2 · one pass · 216 µs
  depth    ██████████████████████████████████████▊    266 × 266 · 283 024 B
```

- Every number is written when the reply lands: request bytes from the encode, reply bytes from each output's `count × 4`, shapes from `EMB.INFO`, times from the bridge, `cached` from the page's own measurement. One scale, so the pooled vector is visibly half a percent of the grid.
- Each reply is a native 44-px `<button>` that jumps to the figure reading it (patches and pooled → the token strip, depth → the stage). No command, no loop, nothing to settle under reduced motion.
- The two heads are drawn **in sequence**, because the run is sequential.

**The grouping gets a figure, and the figure prints the baseline that embarrasses it.** The council's decisive measurement: a **content-blind 8-horizontal-band cut of the same 14 × 14 grid scores 0.731** on the four-neighbour coherence test, while the page's seeded k-means scores 0.670–0.766 — better than the ruler on one picture of six. So the grouping is not a headline; it is a figure that shows what the model groups *and* what a ruler would do with the same lattice:

- **The mosaic:** every patch replaced by the crop of its group's centre patch, cut from the same photograph and repeated. A lookup, not a reconstruction, and the caption says so. One offscreen canvas, 196 `drawImage` calls per sample, byte-identical on a second draw, no call.
- **The ruler toggle:** a native checkbox swaps the mosaic for eight horizontal bands; the two coherences print side by side.
- **The unflattering numbers, printed:** each group's member count (6–41 cells), the 4-connected pieces per group (1–5 — a category does not arrive in five pieces), and the closest two group centres (0.935 on the canyon; median 0.643–0.832, max 0.904–0.958 across the six). A detector never reports that two of its categories are 96% the same.
- **The medoid is not a prototype:** the closest member beats the second-best by 0.001–0.005, so the caption says *nearest its group's own mean* and claims nothing more.
- **The click is the probe's:** a mosaic cell pins that patch; the readout gains `part k · n patches · centre #j`. No sixth control.
- **The legend stays** as eight swatches, because it keys the relief's vertex colours in the `parts` reading, which a mosaic of crops cannot do.

**The correspondence field stays, and stays first.** It is the only claim a trivial baseline could not reproduce: same place **0.798** (0.784 reversed, 0.788/0.800 live), floor **0.706** for the asking picture, **79–81% of the second picture's cells chosen by nothing**, same composition **0.492**, unrelated **0.371–0.556** — and on an unrelated pair it draws nothing and says so. The floor is now computed per picture (`selfFloor`) because the matrix that used to supply it is gone.

**What was explored and rejected, with the number that killed it** (all measured on the shipped replies; D35): direction search (0% of 400 random queries clear the picture's floor; the top cell is the input patch itself); typicality (88–97% of its variance is between the existing clusters); eight prototype tiles (medoid beats second member by 0.001–0.005); feature-space map (PC1 is the image's row axis, ρ 0.28–0.95); exemplar-guided depth (the cosine weighting is a no-op: mean |Δrank| 0.0006–0.0119 against a photo-only filter).

**Shared rules for the figures.** Canvas 2D at `devicePixelRatio` with `image-rendering: pixelated`; animate only while on screen (IntersectionObserver); every interactive part is a native control at 44 px or carries `tabindex` with a keyboard handler; each canvas is `role="img"` with the numbers in its label; `prefers-reduced-motion` draws the settled state in one frame; and the detector's contract holds — the plate measures **8** findings after the rework, all shared with the shell (it was 11 before).

#### 3. The copy is reworked, and the plate is called "The perception"

- **The name** is **The perception** — sentence case, like a plate title rather than a proper noun — in the page's `<title>` and metadata, the head's `VII. The perception`, the gallery's index entry, the gallery's own list of plates and the surface brief. The route, the rig selector and the plate number are unchanged.
- **The copy** is rewritten to the figures: `WHAT YOU ARE LOOKING AT` gains one sentence on the four depth mechanisms; `WHAT JUST HAPPENED` carries the three figures with their captions and readouts; `WHY IT MATTERS` states the ladder (same place 0.78, same architecture 0.66, unrelated 0.39–0.58) and why a dense field is worth more than a label; `THE EXACT COMMANDS` is unchanged except for the library's new filenames. Every number in the copy is one the page printed: a declared shape from `EMB.INFO`, a reply's byte count, a cosine the page computed, or a measurement recorded in this change.
- **The honesty list gains two entries**: the field is dense for the same place, partly dense for the same architecture and sparse for unrelated pictures, and it never claims identity; and the eight clusters are the model's own distances, with no label and no head.
- **The gallery entry** keeps `PLATE VII` and its teaching line is rewritten to the figures and the ladder.

#### 4. What stays exactly as it is

The server's `map` selector, the depth model, the bridge's two-name allowlist, the reveal and its clock, the three readings, the probe and the grown region, the crop and its graded answer, the plan as the accessible and WebGL-free figure, and the four sections' structure. The rework adds figures and photographs; it removes no capability.

#### 5. Gates this rework must pass before it ships

1. **Depth gate:** every shipped picture spread ≥ 0.60, both deciles ≥ 0.01, gradient ≥ 0.006 — measured, table above.
2. **Structure gate:** every shipped picture's 4-neighbour cluster coherence ≥ 0.60 at k = 8.
3. **Ladder gate:** the same-place pair ≥ 0.75 median and ≥ 0.60 pooled; the composition pair is *not* required to separate (measurement says it does not), and the unrelated floor stays ≤ 0.45 median. Measured: same place 0.798/0.678, composition 0.492, everything else 0.371–0.556.
4. **Crop gate, re-measured on the new library:** at least 7 of 9 crops land inside their own box, and the source picture ranks first for at least 7 of 9; a crop whose best picture is not its source prints both lines rather than a claim. Measured on the six: **9/9 inside**, 4/9 at rank 1, **9/9 source-first**, against 8/9 for the plain mean — so the canyon pair does not steal a canyon crop's own place.
5. **Figure gate:** an interaction with the trace or any figure issues **no** request (measured: 0 commands on a mosaic click, a field reverse and a ruler toggle); the mosaic and the strip are byte-identical on two draws of one sample; the figures hold their frame budget with the stage orbiting (**61 fps** measured); reduced motion draws them settled in one frame; every control is a native 44-px target reachable by keyboard.
6. **Site gate:** `node --check`, `website-pages-check` (after a re-stamp for the new name and description), `website-presets-check` (no preset changes), `website-published` with the six new paths and the six old ones gone, `website-ink` at all 24 widths, and a browser pass at desktop and phone widths with an empty console.

#### 6. Non-goals, restated for the rework

No text tower, no cross-modal claim, no semantic labels, no segmentation head, no server-side index, no stored figure or precomputed matrix, no WebGL for the figures (canvas 2D is enough and cheaper), no stock-site hotlinks, and no autoplay that a reader cannot stop.

### Priority boundary

- **P0, in this change:** one accurate depth map → a dense, recognisable, depth-shaded relief → a cinematic, reversible reveal → the site's own design system.
- **P1, in this change:** the depth map shown as itself, a dragged profile in the shader's own display units, and a depth of field with its sharpness measured, under the plate's sentence-case name **The perception**.
- **P2, deliberately out:** on-device ONNX for uploaded images (a second wasm runtime, a model download and a worker path; an upload travels to the sandbox's one model instead) and a bloom post pass. DINOv3, its patch features and every figure built on them are removed rather than deferred: they were measured against a ruler and a cosine null and did not survive (D33–D35).

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `embedding-demos`: the plate is a stage that leads with its figure — **one depth model**, one photograph, one 266 × 266 relative map turned into a lit relief, and three ways to read that same map: the reply drawn beside the picture (`raw` / `rank-equalised`), a dragged profile in the shader's own display units, and a depth of field that refocuses the photograph by depth with a measured sharpness ratio. DINOv3's patch features, the parts reading, the probe, the crop, the correspondence field and the six-picture ranking are deleted from the plate; the photograph keeps its own scale, the reveal stays, the controls stay quiet, the long form stays behind disclosures, and the map canvas remains the accessible and WebGL-free figure. The plate is named **The perception** wherever the site names it.
- `image-embeddings`: a new `select: map` output kind for 2-D tensors, reported by `EMB.INFO`, so a depth map reaches a client as packed float32 with its shape.
- `sandbox-service`: the fixed surface admits **one** image model (`depth`); `dinov3` is removed from the model set and from the bridge's allowlist, and the refusal names the one model that remains.

## Impact

- **Server**: nothing. The `map` and `tokens` selectors already ship, and their tests stay. Deleting the plate's second model is a site and sandbox change only.
- **Sandbox**: `website/repl/sandbox.yaml` (the `dinov3` entry deleted, the `depth` entry kept), `website/repl/allowlist.go` (one image model), the bridge's table tests and refusal texts. `website/repl/bridge.go`'s shape is unchanged.
- **Site**: `website/demos/perception.html` (the stage, the reveal, the two readings, the map, the profile, the focus, the one-lane trace, the sections, the disclosures), `website/assets/css/styles.css` (the stage, the map, the profile chart, the focus controls, the disclosures), `website/assets/js/demos.js` (one map decoder beside `image()`), `website/demos/samples/parts/` (six Unsplash photographs through their Commons CC0 mirrors, plus `CREDITS.md`'s Unsplash columns), `website/tools/published-tree.py`, `website/tools/pages.json` and the stamped metadata, and `website/.impeccable/surfaces/website-demos-perception-html.md`.
- **Docs**: `docs/image-models.md` (`select: map`, the depth model, the reply's weight), `docs/commands.md` if the output kinds are listed there.
- **Unchanged**: the image decode path, preprocessing, cache identity, the scripting surface, and every other plate.
- **Licensing**: DINOv3 weights under the DINOv3 License; Depth Anything V2 Small under Apache-2.0; both downloaded by the sandbox at boot, never committed. Photographs are PD/CC0 with their Commons page recorded — including the Unsplash originals that arrive as their Commons CC0 mirrors, which is how the tree already handles them. three.js stays MIT.
- **Measurement**: **one call per sample** (`EMB.IMG depth BLOB OUTPUTS depth`), cached by content hash; one vertex buffer feeding two readings; 48 B per vertex at 513 792 vertices for a 768×669 sample (about 25 MB); the map figure is one 266 × 266 raster, the profile is 266 bilinear samples per drag, the focus is six banded draws with `ctx.filter` on a 768-px canvas (budget ≤ 40 ms) plus a Laplacian variance on a 192-px copy; no figure issues a call, and every one draws its settled state in one frame under reduced motion. The sandbox's resident set drops by the DINOv3 sessions.
