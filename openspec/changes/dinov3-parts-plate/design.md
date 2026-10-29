# Design

## Context

See `proposal.md` — Why. The server already serves DINOv3's two products from one graph run (`openspec/changes/dinov3-image-outputs`, complete), the bridge admits one shaped image command (`website/repl/allowlist.go`), and the gallery has a working image plate. What does not exist is **depth**: the sandbox has no depth model, and the server's image outputs can only reduce token rows (`none`, `cls`, `mean`, `tokens`), so a 2-D map has nowhere to go.

The figures the plate must produce are bounded by what the model reports and by the photograph the browser sends:

- the DINOv3 grid: 196 rows × 384 `float32` (301 056 B) plus a 384-float pooled vector;
- the depth map: one `float32` per pixel of the depth model's output (266 × 266 = 70 756 values, 283 024 B packed; 377 368 B base64);
- the photograph: bounded to 768 px on the long edge (~40–120 KB JPEG), which is also the particle grid (768 × 512 = 393 216 particles for 3:2; 768 × 768 = 589 824 at most).

## Goals / Non-Goals

**Goals:**
- One photograph becomes a volumetric, photographically recognisable particle field, then the same particles become DINOv3's feature space, on one continuous control.
- The photograph's own scale and colour survive every state.
- **The figure is the page**: the first screen is the stage, the photograph revealing itself, and the prose is one disclosure away.
- The smallest server and surface change that makes a depth map reachable, with the map's shape declared rather than guessed.
- A figure a screen reader and a WebGL-less browser can still read.

**Non-Goals:**
- Two images per call, a corpus, URL fetching, or user uploads with browser-side inference (the plate uses its committed samples; an upload path is a separate change).
- A depth *head* trained on DINOv3 (the published DPT heads are not served); the plate uses a separate monocular model and says so.
- Semantic labels, segmentation, or any classifier: similarity is the model's own cosine.
- Invented geometry: no backside, no hole filling, no claim of complete 3-D reconstruction.
- The brief's P2: an on-device ONNX worker for uploads, cross-image correspondence, a normals overlay, and a bloom post pass.

## Decisions

### D1. Two calls per sample, one particle field

```
sample .jpg --bounded(768)--> ~60 KB JPEG
      |
      | EMB.IMG dinov3 BLOB OUTPUTS embedding,patches <bytes>   -> 196x384 f32, 384 f32
      | EMB.IMG depth  BLOB OUTPUTS depth            <bytes>   -> 266x266 f32 (relative)
      v
  per particle (one per bounded-image pixel):
     colour  <- the photograph's own pixels (canvas)
     flat    <- image proportions, aspect-true
     world   <- flat + depth(u,v) * exaggeration
     slope   <- the depth map's gradient (the relief's normal)
  one Points, three position attributes, one similarity texture
```

- Both calls are content-hash cached by the server, so a repeat of one sample is two lookups.
- The plate does not ship a precomputed depth map or feature grid. The brief asks for committed assets so the featured example renders instantly; the gallery's contract says every figure is computed from the live reply, so the plate pays the calls and prints their measured times instead. A cached repeat is the fast path.
- Hover never calls: the probe is local, and pressing a mode is a uniform change.

Rejected: deriving "depth" from DINOv3's own principal component (that is a feature axis, not a metric, and the brief is explicit that depth needs a trained decoder); a second model call per probe; `VALUES` for the depth map (268 k JSON numbers).

### D2. The server gains one output kind: `map`

```yaml
depth:
  onnx: /data/models/int8/depth/model_quantized.onnx
  quantize: auto
  image:
    preset: imagenet
    size: 266
    outputs:
      - {name: depth, tensor: predicted_depth, select: map}
```

- `select: map` accepts a rank-3 `[1,H,W]` or rank-4 `[1,1,H,W]` tensor, emits `H*W` packed `float32` in row-major order, and reports `rows: H`, `dim: W` in `EMB.INFO`.
- A map is **not normalized**: depth is not a unit vector, and the client owns the transform. `start`/`grid` are invalid with `map` and fail at boot; `VALUES` on a map is refused at request time (a 266² decimal reply is not a thing the sandbox should build).
- The tensor name is confirmed against the graph at boot; the boot fails loudly if it is wrong rather than returning an empty map.
- 266 is the chosen edge (19 × 14, so the export's multiple-of-14 constraint holds): 283 024 B packed, 377 368 B base64 — 1.3× the patch grid's reply, not 3.6× the 518 dialect's. The depth map is upsampled bilinearly in the page, where the particle grid is 768 wide; the extra depth resolution would not survive the point size.

Rejected: 518 (1.43 MB base64 for detail the particle field cannot show); asking the depth model for a pooled scalar (no parallax); a browser-side ONNX runtime (a second wasm payload, and the sandbox already has the model).

### D3. The bridge admits the sandbox's two image models, nothing wider

```
EMB.IMG <dinov3|depth> BLOB [OUTPUTS <name>[,<name>…]] <one image>
  dinov3 -> embedding, patches      depth -> depth
```

Everything else stays refused, and the refusal names the permitted form. The binary-argument gate and `wantsBlob` are unchanged: the command's shape decides that its bulks are binary, not the bytes. The two names are configuration, not a family: adding a third image model is a config-and-allowlist change, and the test asserts the exact pair.

### D4. Every number on the page comes from the server

Before the first draw the plate reads `EMB.INFO` for both models (memoized once per page): `patches` gives `rows`, `dim`, `grid`; `depth` gives `rows`, `dim` and its kind. The depth reply's element count must equal `rows × dim`, or the plate states the disagreement and draws nothing. The patch grid's count must equal `rows × dim` and `grid² == rows`. Prose describes the models' architecture; any sentence about *this* reply's shape is derived.

### D5. The particle field keeps the photograph's scale

- **Sampling.** One particle per pixel of the bounded image (768 long edge), capped at 600 000. Colour is read from the same canvas the browser bounded. A 3:2 photograph gives 393 216 particles; a square one 589 824.
- **Flat positions.** `x = (u − ½) · (W/H)`, `y = ½ − v`, `z = 0`, so the field's extent is the photograph's aspect, not a square. The world box is normalised so the long edge is 1.
- **World positions.** Pinhole back-projection with an approximated focal: `X = (u − ½) · aspect · Z`, `Y = (½ − v) · Z`, `Z = 1 + (d̂ − ½) · exaggeration · 0.18`, where `d̂` is the depth map normalised by its 1st/99th percentiles **and then equalised by rank** (D17) and `exaggeration` is the control. The 0.18 keeps the parallax at about a fifth of the frame's height: enough to read near from far, not enough to tear the picture apart. Depth is inverse relative depth (larger = nearer).
- **Mind positions.** The 196 patch rows are projected to three dimensions by PCA (Gram matrix, power iteration with deflation against the eigenvectors, deterministic start and sign). Each particle takes its patch's coordinates and adds its own sub-patch offset `((u·grid − col) − ½, (v·grid − row) − ½) · 0.08` — no random number generator, so the field is identical on every visit and a screenshot means something.
- **Attributes.** `aFlat`, `aMind`, `color` (3 each), `aPatch`, `aDepth` and `aSeed` (1 each): 12 floats, 48 B per particle, 19–25 MB for the whole field. One draw call.
- **Similarity.** A 196-float `DataTexture` holds the probe's cosine to every patch; the vertex shader reads it by `patch`. Pressing the probe uploads 784 B, not 393 k vertices.

Rejected: a second particle per pixel for "density" (the brief's density is the photograph's resolution); random jitter between states (identity must be stable); a per-particle `THREE.Points` material per state (two draws, two uploads, two sources of drift).

### D6. One shader, two states, one reveal

```
uRise   0 = the photograph  1 = the relief
vertex  p = mix(aFlat, aWorld, clamp(uRise * (1 + d) − d))    d = seed stagger
        n = normalize(vec3(−slope.x · uExag, −slope.y · uExag, 1))
        shade = 0.28 + 0.82·max(dot(n, key), 0) + 0.22·max(dot(n, fill), 0)
fragment soft disc (smoothstep on gl_PointCoord), sampled colour × shade × haze,
         dim = mix(1, similarity, uHighlight)
```

- The `<img>` stays under the canvas and fades out as `uRise` leaves 0, so the photograph hands over to flat particles as a crossfade between two pixel-aligned renderings, not a pixelated dissolve.
- The reveal runs on one clock and starts per asset (D13). Reduced motion renders the settled relief in one frame. The vertex shader offsets each particle's mix by a deterministic `aSeed`, so the dissolve travels instead of snapping.
- The camera is constrained: yaw ±25°, pitch ±15°, wheel 1.2–6×, double-click resets, and no orbit but the reveal's own 12.6° swing. Extreme angles expose the reconstruction's missing backside, so they are not allowed — ±25° is enough parallax to read three planes and not enough to open the hole.

Rejected: CSS transitions on canvas opacity alone (the point size and stagger need the shader); additive bloom as a post pass (a second render target and a blur for a 60 fps target the plate does not need — the shading is the cheaper read); orbit controls with damping (a frame loop for an orbit the reveal already runs and the reader can take over).

### D7. One probe, in every state

A click on the photograph or a cell in the plan picks a patch (the plan keeps the exact cell math); the row/column controls give the probe a keyboard route. Similarity is a dot product against the patch rows because the server normalized them; the plate does not re-normalize. The probe's own patch is marked, and the readout names the cell, the best other cell and the cosine in every state. No labels, no classes: the copy says the highlight is the model's similarity, not a semantic match.

### D8. Six photographs, their own scale, their own credit

- The plate gets its own sample set in `website/demos/samples/parts/`, so the image plate's artworks are untouched. The six are chosen by the depth their own maps carry, not by subject: every candidate goes through the plate's depth model, its 266×266 map is min–max normalised and reduced to the p5/p95 spread, the near- and far-decile occupancy and the gradient energy, and only frames whose spread reaches both ends of the range are kept — which is what excludes the flat, the night and the all-far-field frames, whatever they show. In filmstrip order, the first as the default: a curving autumn road (Aaron Burden, CC0, 768×576), the Lincoln Cathedral nave (Michael D Beckwith, CC0, 768×512), an autumn forest path (Vovogov90, CC0, 768×512), a castle courtyard looking out (Michał Bulsa, CC0, 577×768), the Yellowstone River in its canyon (Jacob W. Frank / NPS, public domain, 768×512) and the Seven Arches aqueduct (Chemical Engineer, public domain, 768×654).
- Each is public domain or CC0 from Wikimedia Commons, committed at a 768-pixel long edge in its native aspect, with a row in `samples/CREDITS.md` naming the work, the author, the licence and the Commons page.
- The picker letterboxes (`object-fit: contain`) instead of cropping, so the thumbnails show each photograph's real shape; the bounding step keeps the aspect, and the scene, the plan and the particle grid all derive their proportions from the same image.

Rejected: reusing the six paintings (flat, engraved, low depth separation — the brief's whole point is photographs with layers); a direct Unsplash import (its licence text would have to enter the tree and the credits are weaker than a Commons page — the one Unsplash original that ships arrives as its Commons CC0 mirror, credited there); crops per sample (the scale is the point).

### D9. The sandbox's model set, with the depth model accounted for

```yaml
dinov3:
  model_repo: onnx-community/dinov3-vits16-pretrain-lvd1689m-ONNX
  workers: 1
  onnx: /data/models/int8/dinov3/model.onnx
  quantize: auto                      # the repo's pre-quantized member, with its sidecar
  intra_op_threads: 4
  image:
    preset: dinov3
    size: 224                         # the export's own preprocessor_config.json
    outputs:
      - {name: embedding, tensor: pooler_output}
      - {name: patches, tensor: last_hidden_state, select: tokens}

depth:
  model_repo: onnx-community/depth-anything-v2-small
  workers: 1
  onnx: /data/models/int8/depth/model_quantized.onnx
  quantize: auto                      # the repo's int8 member, 27 MB on disk
  intra_op_threads: 4
  image:
    preset: imagenet
    size: 266
    outputs:
      - {name: depth, tensor: predicted_depth, select: map}
```

- Depth Anything V2 Small is 25 M parameters, Apache-2.0; after the CPU provider dequantizes it, the resident weight is on the order of 100 MB — noise beside `laya-real`'s ~2.3 GB and the DINOv3 export's ~90 MB.
- The image preprocessor's `keep_aspect_ratio` hint is not implemented by emb; the square squash is harmless here because the page samples the map by normalised `(u,v)`, which survives any uniform or non-uniform resize.
- Measured on the development host (2026-10-28, 4 intra-op threads): a cold depth call through the bridge answers in **265 ms**, a repeat in **194 µs**, and the map is byte-identical between the two. The features call beside it is 131–172 ms cold. The plate prints the server's measured times; if the deployed host exceeds the deadline, `size` drops to 224 (16 × 14) before `intra_op_threads` changes.

Rejected: a DINOv3 DPT head (no usable ONNX export; the linear depth probes are not served); Depth Anything V2 Base (4× the compute for a parallax the particle field cannot resolve).

### D10. The plan is still the accessible figure, and the only one without WebGL

- The plan canvas paints the photograph with the grown region's wash at cell resolution, rings the probe's cell and carries the region's count, the best match and the cosine in its label.
- Without WebGL or without the vendored module, the plate states it and draws the plan plus the readout from the same replies — the field is unavailable, the reading is not.
- Every control is a DOM control (the state and reading radios, exaggeration, cut, density, isolate, upload, the probe's column and row), so a keyboard or screen-reader reader moves the same state; the readout is `aria-live` and names the state.
- Nothing animates after the reveal settles; the reveal itself is skipped under reduced motion.

### D11. Gallery integration stays descriptor-driven

The page stays entry 15 in `website/tools/pages.json`; the metadata, social text and `sitemap.xml` are stamped, never typed. The index's teaching line changes to the three states. No preset digest is involved, so `website-presets-check` is untouched.

### D12. The plate is a stage: the figure comes first, the words are one disclosure away

The page's order is the argument. Above the figure: the plate's number and title (the reference's "See beyond the image."), one sentence naming the two models and the three states, and the ruled caption with the sample credits. Under it, immediately: the stage, which reveals itself on arrival, then the filmstrip, the state and reading controls, and the readout. Then, and only then, prose.

```
.plate--stage   title · one sentence · FIG. chips                    ← 177px on a 1440×900
.block--dark.stage-block                                              ← the near-black ground
  .stage__view  .parts__stage   the canvas, the photograph's aspect, 78vh
                .stage__cue     Reveal perception (the replay, shown at the photograph)
                .stage__hud     vertices · reading · cut (one measured line)
  .stage__film  six letterboxed thumbnails, one row, 76px tiles
  .perception__states  ×2  the state radios and the reading radios
  .rig__readout the probe, region, grid, depth, counts and the two times
  .stage__under plan (200px) · adjust (details) · the camera hint
  details.cmds  the commands this run sent
.parts__fold--prose  ×2  the long version · the exact commands
```

- **Text budget.** The surface carries the title, one sentence, three state labels, five reading labels and one readout line. Everything else is inside a `<details>`, which keeps the static page complete for a reader without scripting (`details` is native) and keeps the first screen a photograph.
- **Two live controls.** The state control and the reading control are always visible because they are the interaction; depth exaggeration, the cut, the field density, the isolate toggle, the upload and the probe's column and row are one disclosure. The camera needs no chrome: drag, wheel and double-click, with the hint in the readout line.
- **The probe does not hide.** The plan and the readout stay under the stage: a tap pins a patch and fills them, and the selection survives every state and every frame because it addresses the same patch index. The probe's column and row are the keyboard route inside the disclosure.
- **Reduced chrome, not reduced honesty.** The disclosures carry what the earlier version put on the surface — the two calls, the shapes, the cache, the one-face limit, the licences — so the page loses nothing a reader can check.

Rejected: keeping the prose above the canvas (the earlier layout; it reads as an article about a figure, which is what the brief forbids); a card grid of "related demos" (the brief is one image, not a dashboard); six always-visible mini-visualisations (the plan and the probe carry the reading; normals and a constellation strip are P2).

### D13. The reveal: three phases, one clock, started per asset

```
t (ms)      0 ──── 900 ────── 3900 ─────────── 6100
uRise       0 → 0.12     → 1                      (photograph → flat field → relief)
uLight      0 ───────────────────────→ 1          (the gradient's light and the haze)
camera.yaw  0 ───────────────────────→ −0.22 rad (−12.6°)
```

- One `performance.now()` baseline, one `requestAnimationFrame` loop, three eased phases: a 900 ms dissolve that only drives `uRise` past the plane's crossfade, so the photograph hands over to its own pixels instead of exploding; a 3 000 ms depth phase that carries `uRise` the rest of the way; and a 2 200 ms orbit.
- `applyLook` carries **three** scalars — `rise`, `mind`, `light` — and `light` is the camera phase. The depth map's gradient is the light, so it is zero for the first 3.9 s and the lift is seen as the still's own displacement while it travels; `vShade` and `vFade` both sit behind `uLight`, so the shading and the distance falloff arrive together as the camera turns.
- The plate starts it itself. `run` draws the replies, then, when the state is still the photograph, calls `reveal()`; a sample switch resets the state to that asset's photograph first (`S.view = 0`, the photograph radio checked), so every switch re-reveals from its own still, and an upload reveals the same way.
- The cue is the **replay**, not the way in: `syncCue()` hides it while any state but the photograph shows, returns it when the reader picks the photograph, and pressing it runs the sequence again.
- Any pointer press, wheel, double-click or key event cancels it, as do the state and reading controls and the exaggeration slider; the state belongs to the reader afterwards.
- `syncFigure()` runs from every `applyLook` and `applyMode`, so the mesh's visibility follows `rise` per frame. That is a fix, not a detail: while it was only set when a state or a reading changed, the mesh stayed off for the whole sequence and the transition hid behind a photograph fading out.
- Reduced motion skips the loop entirely: `applyLook(1, 0, 1)` and the reveal's angle in one render.
- The camera reveal ends by handing over to direct manipulation at the reveal's own angle, not at zero — the reader sees what moved and can keep turning from there.

Rejected: a CSS transition of `uRise` (the phase boundaries, the seed stagger and the cancellation need the loop); looping the reveal (it runs once per asset, and only the cue repeats it); leaving the reader to find one action before anything moves (the plate is the figure, and the cue stays for the second look).

### D17. The depth is equalised by rank before anything uses it

The first version normalised the depth map by its 1st/99th percentiles and used that value directly. A browser pass over the live replies showed why that is not enough: **on a narrow street most of the frame is near**, so after the percentile stretch nearly every particle sat at the near end — the field rendered as a slightly bent card and the lamp washed out to a constant. The live attribute was measured at `lo 0.04, hi 1.00, mean 0.50` only *after* the fix, with every decile of the picture holding 10% of the particles.

- `depthRankMap()` builds the map's cumulative distribution in 256 bins once per sample; `sampleDepth()` maps each bilinear sample through it. The ordering is the model's own; the spacing is even, which is what a relief needs.
- The rank feeds the geometry *and* the light, so both separate across the whole picture instead of only at the extremes.
- It is a monotone remap of a relative quantity, and the copy says the depth is relative: nothing here claims metres.

Rejected: leaving the percentile stretch and compensating in the lamp (the geometry stayed flat — the actual defect); equalising only the lamp (the volume would still be a card).

### D14. (retired) The colour modes

The feature-colour mode existed to dress the PCA projection. With the projection gone the mode has no subject, so it was deleted with it: the `aFeatureColour` attribute, the `uColourMode` uniform, the patch PCA (`principalAxes`, its Gram matrix, its deflation) and the button all went. The photograph's colours and the model's cosine are the two colour languages left, and each has a job.

### D15. The sprites are measured in CSS pixels, and the stagger never overshoots

- `gl_PointSize` is in framebuffer pixels, so a clamp written in device pixels gives half the sprite on a 2× display: the first browser pass drew a sparse field that did not read as a photograph. The shader clamps the size in **CSS** pixels (1.2–2.8) and multiplies by `uDPR`, and `uFocal` is the canvas's CSS height, so a 2× display draws the same picture (POINT_SIZE 0.0036 ≈ 2 px at the fitted distance).
- The per-particle dissolve offset is folded into each leg, `k = t + d·(t − 1)`, so it is exactly 0 at both ends of both legs. The earlier form subtracted the offset from `t` and read past 1, which pulled a quarter of the particles toward the feature space while the reader was in WORLD — the "world" state was measurably not the world.
- `fit()` fits both axes (photo aspect against the stage's) with a 1.16 margin: enough that the reveal's 12.6° orbit keeps the whole photograph inside the frame, and 86% of the frame for the picture itself. The zoom clamp follows the fitted distance (0.6×–2.6×), so a sample's own size cannot arrive outside the range.

### D16. The depth also lights the field

```
haze   = clamp((dist − 0.95·fit) / (0.12·fit), 0, 1)   // camera distance
fade   = mix(1, 1 − 0.35·haze, uRise)                   // distance falls away
shade  = D18: the normal-map key + fill, scaled by uRise
```

- The reconstruction reads as a photograph and, head-on, only the sorting of the parallax tells you it has depth. So the depth model's own ordering also drives light: `aDepth` 1 (nearest) gains 50%, `aDepth` 0 (farthest) loses 30%. It is the brief's "atmosphere and depth fading", it costs one varying and one multiply, and it changes no hue, so the photograph's colours survive.
- `uLight` is zero at the still, which is what keeps IMAGE exactly the source: the same frame, the same pixels, no lamp.
- The light is the depth's own gradient once the field is raised (D18); the brightness ramp survives only as the distance haze.
- **Seasoning for the eye, each bounded.** An atmospheric tint (near warmer, far cooler, ±6% a channel) is the one cue a photograph's own lighting cannot fake, and a 7% scatter glow on the nearest fifth reads as light in the air rather than as a filter. Both are zero at `uLight = 0`.
- The lamp is the only added lighting; no post pass, no second render target, no bloom kernel.

Rejected: a key light with invented normals (there are none — a depth map is one face, and shading it as a surface would invent geometry); a colour ramp for depth (recolours the photograph, which the brief forbids); lighting the still state for consistency (it would break the IMAGE/WORLD contract that the first state is the photograph).

### D18. The depth map is the normal map: the relief is a lit surface, not a brighter picture

A luminance ramp on a photograph competes with the photograph's own light, and it lost: a live A/B on the same frame moved the near band by single-digit percent while the eye read neither frame as lit. The honest fix is to light a *surface*.

```
height = (rank(depth) − 0.5) · 0.18, blurred 5×5 once            // CPU, per sample
slope  = (∂h/∂x, ∂h/∂y) in world units                          // CPU, per cell
n      = normalize(vec3(−slope.x · uExag · 1.35, −slope.y · uExag · 1.35, 1))
shade  = 0.34 + 0.76 · max(dot(n, key), 0) + 0.20 · max(dot(n, fill), 0)
```

- The slope is the map's own gradient at the exaggeration in force (from a 5×5-blurred height), so the **exaggeration control changes the light as well as the lift** — dialling the relief to zero flattens the shading to a constant, which is the invariant that proves the normals are doing the work.
- The key is fixed in world space (upper left, in front) and the fill from the opposite side, so orbiting moves the highlight across the facades the way a lamp does.
- The height is blurred 3×3 before it is differentiated: a monocular map is noisy at the cell level, and differentiating its noise gives a stipple rather than a surface. The blur radius and the gain were calibrated on the two hardest scenes of the set as it stood then: at 3×3 and 1.6 the near hoarding of the night street stippled; at 5×5 and 1.35 the column mouldings of the nave still catch the key and the street's facades hold their form. The night street has since been replaced (D8), so those numbers are recorded history, not a claim about today's default.
- One `vec2` attribute (`aSlope`, 8 B/particle) and five multiply-adds per vertex. No post pass, no shadow map, no invented geometry: a depth map has exactly one surface, and this is its gradient.

Rejected: inventing normals from luminance (they would shade the paint, not the form); a specular term (the particles are points, not facets — a second light lobe cost more than it read); keeping the luminance ramp as the main cue (measured, and it did not read).

### D19. The probe searches the other five photographs, and that is the feature

The PCA constellation was decoration: it proved the vectors existed and answered nothing a reader could ask twice. Dense features are a **search index**, so the plate uses them as one.

- Every sample's patch grid is fetched once, lazily, at the first probe, and kept in memory (`gridFor`). Bounding is deterministic, so the server's content-hash cache answers every repeat.
- A probe's vector is a `slice` of the current grid; ranking is one dot product per row, per picture, in the browser. Six grids are 6 × 196 × 384 floats — 1.8 MB, no index, no store.
- The strip prints the best cell and the cosine per picture, sorted; a result opens that photograph and pins the patch the model chose. Hovering re-ranks, so the answer follows the pointer, debounced to 160 ms.
- The copy is explicit that a high cosine in another picture is a *similar patch*, not the same object, and that the numbers are only comparable because one model produced every grid.

Rejected: an on-server ranking command (the vectors are already in the page; a round trip would buy nothing); precomputing per-pair matches for all 196 patches of six pictures (36 864 pairs for a strip a reader will look at twice); a cross-modal probe (no text tower is served).

### D20. The region is grown from what the server already sent, and the field is an instrument

DINOv3 returns no masks, and the plate's third question is "what is this part, and where else is it like this". Two answers are honest and cheap; a segmentation head is neither.

- **Growth, not segmentation.** `growRegion` walks four-neighbour connectivity from the probed patch. A neighbour joins if the seed's cosine clears the **median** of that grid, the depth map stays within `0.02 + 0.45·0.43` of the seed, and the photograph's mean patch colour stays within `0.05 + 0.45·0.85` of it. Both tolerances are constants — no control moves them — so a region is one definition a reader can verify rather than a knob.
- **Two thresholds, because they answer two questions.** The highlight is a steep power of the cosine — the glow — while the growth threshold is the median, because a neighbour is almost never in the top 12% of a patch's own cosines: using the highlight cut for growth produced a one-patch region on every seed. Measured over 28 seeds of the night street, on the set as it stood then — that photograph has since been replaced, and the count is history, not a claim about the shipped default: min 1, median 6, max 38, three single.
- **The query is the region's mean.** `regionQuery` averages the member rows and L2-normalizes, so a boundary patch cannot decide the ranking; the same vector drives the in-picture accent and the cross-picture strip, so a mode never ranks with a different vector than it highlights with.
- **One texture, two channels.** The cosine and the membership are written as `RG` floats into a `rows × 1` `DataTexture` on each press; no particle attribute is rewritten, so a hover costs one upload of 392 floats and the geometry stays as built.
- **Isolation is alpha, and the box is the same arithmetic.** `uIso` leaves members at 1 and everything else at 0.12 — dense sprites accumulate, so the remainder reads as a trace rather than vanishing, which is what a reconstruction with one face can honestly offer. `drawBox` repeats the vertex shader's `flat·(1+z), z` in JavaScript to build a `Box3`, so the box cannot drift from the vertices it encloses.
- **Normals are a view, not a material.** The vertex shader already builds `n` for the key light; the view paints `n·0.5 + 0.5` and drops the photograph's colour. It raises the relief first, because an orientation only exists once the surface does.
- **Density is a re-sample.** Three tiers (3 · 2 · 1 pixels per step) build the same grid at three resolutions, so a tier change rebuilds one array and issues no command, and the same tier always draws the same field.
- **An upload is the samples' path with different bytes.** The file input feeds `run`, which bounds the image, sends it to the same two models through the same bridge, and stores nothing; `S.id = upload` makes the strip rank the uploaded part against all six committed samples.

Rejected: rectangle-drag selection (the grown region is the selection, and a rectangle adds a mode to the only gesture the stage has); a movable light (a second light is a lighting rig, not a view of the data); region growing that re-anchors on the running mean (it can oscillate between two regions and has no fixed point to verify); on-device ONNX for uploads (a second runtime, a model download and a worker — the sandbox already answers with the served graphs).

### D21. One indexed surface, three renderers, and the readings are uniforms

The plate's figure was a field of points. The brief asks for a scan: a mesh you can wireframe, shade by orientation, contour by depth, cut, and read as a cloud. All of it has to be one reconstruction, so the geometry is built once and every view is a uniform or an index over it.

- **A regular grid, not the hash sample.** The cloud used to keep pixels by a deterministic hash. A hash sample cannot be indexed (the compacted buffer has no grid relationship), so the density tiers became a regular pixel stride (`3 · 2 · 1`), and one grid feeds the mesh and the cloud. That is also why the tiers now change the mesh resolution, which the brief asks for anyway.
- **Two triangles per cell, cut before the index exists.** A cell survives only when its four edges and its diagonal each move the display depth by less than the tolerance; the triangles and the wireframe edges are generated from the survivors, so the wireframe cannot draw an edge the surface does not have. At the default 1.0% of cells are refused at full density, 5.0% at the coarse tier — a three-pixel step crosses more edges, which is the honest consequence of asking a coarse grid to resolve fine depth.
- **Three renderers, one buffer.** `Mesh` (triangle index), `LineSegments` (edge index, 0.0025 lift to stop the z-fight) and `Points` share the same `BufferAttribute` instances; only the index differs, and each material owns the three uniforms that cannot be shared (`uLift`, `uOpacity`, and the per-object identity). A view change is therefore a uniform write, never a rebuild.
- **The wireframe is a stride, not every edge.** At 768 vertices across, every edge merges into a white sheet. The lattice draws every `round(width/96)`-th grid line plus each stride cell's diagonal, so the topology is legible and every drawn line is a real edge. A barycentric wireframe was rejected: it needs a non-indexed geometry and three vertices per corner — three times the memory for a line that is 1 px wide anyway.
- **View depth is inverted once.** The model's ordering runs the other way from the reader's words, so the vertex shader computes `viewD = 1 − depth` and the contours and the scan speak in `viewD`. Before that, `near` kept the far end.
- **The pick is analytic, not a raycast against the canonical mesh.** The drawn surface depends on the reveal and on the exaggeration, so a mesh raycast would be exact only at one setting. The surface is a height field, so the ray is walked onto it with an eight-round fixed point over the same constants the shader uses: exact at every setting, no pick mesh, no re-bake.
- **The still is still the photograph.** A refused cell is a hole, and at rest a hole in the picture is damage. The exact image is drawn over the surface at rest and kept as a ghost behind it afterwards; it leaves with the cloud reading and with isolation, which are the states that claim to be geometry only.
- **The normals are the map's gradient, clamped.** `computeVertexNormals` on the accepted index would smooth across the very cells the cut removed, and the unclamped gradient turns every depth cliff into a black band. The clamp keeps a cliff a steep ramp, which is what a monocular map can honestly support.
- **Measured:** 513 792 vertices / 1 014 698 triangles / 135 600 wire edges at full density, rebuild 380 ms; 62 frames/s with contours animating and the cloud at 45%. The nave sample uploads to 393 216 vertices and 780 970 triangles with 0.4% cut.

## Risks / Trade-offs

- **A depth map mis-shaped or mis-typed** → `select: map` reports its shape in `EMB.INFO`, the page refuses to draw when `count ≠ rows × dim`, and a bridge test returns a map whose bytes are valid UTF-8 to prove the binary rule is command-shaped.
- **A 377 KB reply per sample on a shared machine** → one model, one map, content-hash cached; the plate prints the measured time and never claims a latency.
- **A wrong depth sign or normalisation** → the exaggeration control goes to zero, which must flatten to the photograph exactly; that is the invariant the smoke check asserts.
- **Extreme angles expose holes** → the camera is clamped, and the copy says the reconstruction has one face.
- **Particles read as a trick, not the photograph** → colours come from the pixels, not a ramp; the flat state is pixel-aligned with the photograph; density is one particle per pixel.
- **Photo licensing** → four CC0 and two public domain, each on a Commons page recorded per file; the one Unsplash original is the Commons mirror of a CC0 release, and nothing arrives from a stock site that is not CC0.
- **A square-squashed depth map** → the page samples by normalised coordinates and the relief is aspect-true, so the squash does not reach the figure.
- **A visual page loses the checkable copy** → every paragraph, list and code specimen survives inside a native `<details>`, so the static page (and a screen reader) still carries the two calls, the shapes, the limits and the licences; the text budget applies to the painted surface, not to the document.
- **A full-height stage pushes the reader's controls off-screen on a short phone** → the stage is capped at `360px` there and the state and reading controls, the filmstrip and the readout are ordinary flow content under it (5.10's phone check).
- **The reveal becomes an animation the reader cannot stop** → any pointer press, wheel, double-click or key cancels it, the state and reading controls take it over, reduced motion lands settled with no loop, and the cue replays it on demand; the page is fully usable with the sequence never finishing.
- **A mesh from a monocular map read as a survey** → the copy states the depth is relative and monocular, the box is in display units, the camera is clamped to one face, and the cut shows open edges instead of a filled interior.
- **A hole where an object meets its background** → the cut is a control with its measured refusal printed in the HUD, and the photograph is ghosted behind the surface so a hole reads as separation rather than damage.
- **A shader change that fails to compile** → a redefinition silently produced a black frame with one console error; every shader edit is now checked against the console, and the geometry has three independent renderers so a broken program is visible as a missing mode, not a missing figure.
- **A region that looks like an object** → the copy names the three tolerances, the readout says "region" and the count, and one definition-list entry states that a segmentation is a different model; the box is the region's extent, not a detector's output.
- **An isolation or a cut that hides the answer** → both are reversible from the same disclosure, and both are alpha or index cuts on vertices that already exist, so no view can invent a surface.
- **An upload at the bridge's cap** → the browser bounds every upload to the same 768-pixel sample before it is sent, so an uploaded 40-megapixel photo costs the same bytes as a committed one.
- **A hidden control is an unreachable control** → every tuning control is a native input inside one disclosure, so keyboard and screen-reader readers open it the same way a mouse does.

## Migration Plan

1. Ship the server's `map` selector and the depth model together with the bridge's second model name; deploy the sandbox by hand (`just sandbox-deploy`).
2. Publish the page after the surface answers; a refused command leaves the plate in its stated unavailable state.
3. Rollback is removing the page's descriptor entry and the bridge's depth name; the server's selector and the model are inert without them.
