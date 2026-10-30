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

> **Superseded by D27.** The rework ships six Unsplash photographs (Commons CC0 mirrors) chosen for the depth mechanism and for one real same-place pair. The list below is the set this decision built, kept as the record of the first pass.

- The plate gets its own sample set in `website/demos/samples/parts/`, so the image plate's artworks are untouched. Every candidate goes through the plate's depth model, its 266×266 map is min–max normalised and reduced to the p5/p95 spread, the near- and far-decile occupancy and the gradient energy, and only frames whose spread reaches both ends of the range are kept — which is what excludes the flat, the night and the all-far-field frames, whatever they show. The second pass (D25) added a matchability gate and replaced the weakest tile, so one pair now shares a subject. In filmstrip order, the first as the default: a curving autumn road (Aaron Burden, CC0, 768×576), the Lincoln Cathedral nave (Michael D Beckwith, CC0, 768×512), an autumn forest path (Vovogov90, CC0, 768×512), the courtyard of the Pieskowa Skała castle seen through a near arch (Michał Bulsa, CC0, 576×768), the same courtyard from the floor between the arcades (Michał Bulsa, CC0, 576×768) and the Seven Arches aqueduct (Chemical Engineer, public domain, 768×654). The two courtyard views are the pair: the same place, two frames, measured at a median patch cosine of 0.855 (D25).
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
- **Deployed** (`emb-sandbox`, `shared-cpu-4x`, iad, deployed 2026-10-30, measured through `https://cli.emb.is`): the served shapes are the declared ones — `EMB.INFO dinov3` gives `embedding` (`pooler_output`, 384) and `patches` (`last_hidden_state`, `select: tokens`, 196 × 384), `EMB.INFO depth` gives `predicted_depth` (`select: map`, 266 × 266); `EMB.IMG dinov3 BLOB OUTPUTS patches` answers **301 056 B** and `EMB.IMG depth BLOB OUTPUTS depth` **283 024 B**, both byte-identical on a repeat, and `VALUES` is refused naming both models. The **first call after boot** costs 2.2 s (features) and 2.5 s (depth) — session creation and the dequantized weight copy — which is the cost of a fresh deploy, not of a request. Steady state: **80–107 ms** cold for the features call and **172–222 ms** for the depth map over three unseen samples, and **1.2–6.4 ms** on a content-hash hit. Both are two orders of magnitude inside the bridge's 30 s deadline, so `size` stays 266 and `intra_op_threads` stays 4.
- **Deployed experience** (same host, page driven from `localhost:8080` against `cli.emb.is`): the first sample reports **107 ms + 199 ms**, a switch away and back reports **1.7 ms + 1.1 ms** and the page's own line says `both replies cached`, and the default tier — 442 368 vertices, 869 016 triangles for the 768 × 576 default sample — holds **60 fps** (the compositor's cap) while the camera orbits on an Apple M4 through ANGLE Metal. The frame rate is the reader's GPU, not the sandbox's: the deployed calls are 300 ms of the first sample's 6.1 s reveal, and every later interaction issues no command at all.

- **Deployed after the removal** (`emb-sandbox`, deployed 2026-10-30): the machine carries one image model. `EMB.MODELS` lists twelve entries and no `dinov3`; `EMB.INFO depth` reports `image_sessions: 1`, `predicted_depth`, `select: map`, 266 × 266; the first call after boot costs **824 741 µs** (session creation) and a warmed session infers a fresh picture in **180–229 ms** on the shared CPU, with the reply byte-identical on a repeat. The plate's reveal clock (6.1 s) starts when the reply lands, so the sequence is unaffected.

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

> **Removed by D36.** The plate no longer carries a second model; the probe, the region and the across-picture ranking go with it.

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

### D22. The figure is one mesh, and three of D21's renderers are deleted (supersedes D21's instrument)

The plate went through the brief's instrument phase — wireframe, camera-space normals, view-depth contours, a particle cloud, guide planes — and then shed it. A reader cannot tell three renderers apart from three uniforms over one surface, and every extra renderer is a second shader to break. What ships is one `Mesh` with the cut index and two readings.

- **Deleted:** `LineSegments` and its edge index, the `Points` cloud, the contour sweep and its uniform, the ruled plane and the axis glyphs. `READINGS` is two entries.
- **Kept from D21:** the regular grid, the cut-before-index rule, the analytic pick against the height field, the ghost photograph, the clamped gradient normals, the rank-equalised depth.
- **Measured on the deployed page:** 869 016 triangles for the 768×576 default sample in one draw call at 60 fps while the camera orbits.

### D23. The feature space gets a reading, and it is a clustering of the rows the server already sent

> **Removed by D36.** The grouping survived a ruler only once (0.766 vs 0.731) and is not the plate's question.

The plate's claim is that a vector per patch is a search index nobody had to build, but the features were only ever a number: the depth model owned every pixel of the figure. DINOv3's own reputation is the unsupervised grouping of dense features, and the plate was disclaiming it ("no segmentation head") instead of showing it.

- **How:** k-means over the returned patch rows — k = 8, k-means++ initialisation seeded at a fixed value, 40 iterations, cosine distance on rows that `normalize: true` already made unit vectors. Each vertex takes its patch's cluster colour. It is a third reading beside surface and normals, a uniform over the same buffer, so the cut, isolate, the probe and the camera compose with it.
- **Why it is not a segmentation head:** no labels, no training, no classifier, no second model. The clusters are the model's own distances; the copy says the model was never told what these are, and no class name appears anywhere in the figure.
- **Measured** (deployed sandbox, six samples, k = 8 at 14 × 14): 4-neighbour cluster coherence **0.66–0.80** against a chance level of **0.11–0.21**, so the clusters are contiguous regions rather than noise. On the nave they separate the vault, both arcades, the chairs, the floor and the far altar; on the autumn road they trace the road's own curve; on the aqueduct they find the arches against the stone band.
- **Determinism:** a seeded initialisation and no re-clustering per frame, so two draws of the same sample are identical and a reading change is a uniform write.

### D24. A crop is a query the page can grade

> **Removed by D36.** The crop was the plate's only graded question, and it was graded by the same cosine; with DINOv3 gone the box goes.

"Look-alikes" ranks pictures against each other and has no right answer. A crop has one, because the page drew the box.

- **How:** drag a box on the photograph; the page crops it, bounds it to 768 px and to the bridge's 256 KiB cap, and sends one `EMB.IMG dinov3 BLOB OUTPUTS embedding,patches`. The query is the mean of the crop's **centre 3 × 3 patch rows**, L2-normalised; the answer is the argmax over the full grid. The readout prints the truth cell's rank and whether the answer fell inside the crop's own cells, and the same query ranks the other five photographs.
- **Why the centre block and not the plain mean:** measured over eighteen crops — three boxes on each of the six shipped photographs — the centre 3 × 3 block lands inside its own box **17/18** with **10/18 at rank 1**, and the crop's own picture ranks first **18/18**. The plain mean of every row lands inside **15/18** with **9/18 at rank 1**. On a second sweep with 25–35% boxes instead of 40–60% the centre block falls to **16/18** inside and **7/18** at rank 1, and the plain mean scores 17/18 inside: small boxes are the harder question, which is why the copy states the size the claim works at. Rank 1 is a strict test — a one-cell miss is a different rank — while "inside the box" is the claim the page prints.
- **Cost:** one cold call per crop, **52 ms** through the bridge on the development host (2026-10-30), cached by content hash, so the same box twice is a lookup — measured 0.9 ms on a repeat.
- **Honesty:** the demo states the crop size it works at and does not claim a hit rate it has not measured; a crop below about 15% of the frame is refused by the page with that sentence.

### D25. The library is curated on two axes, and the second one is measured

> **Superseded by D27.** The two gates still hold; the rework's six are chosen on the depth mechanism first and measured on the same two axes, with one real same-place pair by provenance instead of a cosine coincidence.

The first pass chose six photographs by depth alone, and the feature cosines show the cost: median per-patch best cross-picture cosine **0.32–0.67**, pooled cosines **0.13–0.43**. The strip's "0.51, 0.44, 0.37" is mostly the model saying nothing here matches, so the search claim had nothing to stand on.

- **Gate 1, kept:** the depth metrics from task 14.1 — p5/p95 spread, near and far decile occupancy, gradient energy.
- **Gate 2, new:** at least one same-subject pair (two views of one place, or two frames of one material), with a measured median patch cosine for the pair ≥ **0.6** and a pooled cosine ≥ **0.5**; every other picture keeps part structure, coherence ≥ **0.6** at k = 8.
- **Licence path unchanged:** Wikimedia Commons, CC0 or public domain, read from `extmetadata` rather than a page snippet. Unsplash originals arrive as their Commons CC0 mirrors, which is what `autumn-road.jpg` already is and what the first pass's design recorded as the reason not to import from Unsplash directly.
- **Measured after the second pass** (the shipped six, through the bridge on the development host, 2026-10-30; `k = 8`, 14 × 14 grid):

| file | p5 | p95 | spread | near &lt;0.10 | far &gt;0.90 | gradient | coherence |
|---|---|---|---|---|---|---|---|
| autumn-road.jpg | 0.108 | 0.848 | 0.740 | 0.044 | 0.030 | 0.0191 | 0.764 |
| nave.jpg | 0.032 | 0.927 | 0.896 | 0.157 | 0.058 | 0.0094 | 0.646 |
| forest.jpg | 0.000 | 0.907 | 0.907 | 0.251 | 0.054 | 0.0081 | 0.720 |
| castle-courtyard.jpg | 0.061 | 0.896 | 0.835 | 0.327 | 0.045 | 0.0198 | 0.643 |
| castle-courtyard-floor.jpg | 0.000 | 0.875 | 0.875 | 0.102 | 0.038 | 0.0134 | 0.720 |
| seven-arches.jpg | 0.048 | 0.875 | 0.827 | 0.134 | 0.025 | 0.0155 | 0.712 |

- **The pair is the courtyard:** `castle-courtyard` (the near arch, looking out) against `castle-courtyard-floor` (the same courtyard from the floor) measures a median per-patch best cosine of **0.855** and a pooled cosine of **0.801** — both clear of the gate. Every other pair sits between a median of 0.357 and 0.622 and a pooled cosine of −0.101 and +0.318, which is what "the model says these are different places" looks like on this checkpoint.
- **What was replaced:** the Yellowstone River frame, the weakest tile on both axes — its cluster coherence measured **0.577**, below the 0.6 gate. The two new tiles are a Commons CC0 series by one author (Michał Bulsa), so both licences read as CC0 and both credits point at their own Commons page.
- **Rejected:** growing the library to eight or ten (the filmstrip's six tiles are sized for six and every extra picture is another cold call on first visit); a synthetic pair built from a crop of a sample (it would prove nothing about transfer); keeping the Yellowstone frame while adding a pair (seven tiles, and the failing gate would still be shipped).

### D26. The grid stays at 196 unless the 336 export measures cheap enough

The sandbox serves the onnx-community 224 export: 14 × 14 = 196 patches. A cluster at that resolution is a fat block — visible in the parts render — and every crop answer is a cell of 16 pixels.

- The kornia 336 export (21 × 21 = 441 rows, `descriptor` + `tokens`, `start 5`, `grid 21`, fp32, 86 MB) is already supported by the registry and covered by `TestDINOv3KorniaExport`, so the finer grid is a configuration change, not new code.
- **The cost is not assumed.** Tokens ×2.25 and attention roughly ×5 put a cold call at an estimated 300–400 ms against today's measured 80–107 ms. The swap is gated on measuring the 336 export's cold call through the bridge on the deployed host: **≤ 400 ms** and a reply **≤ 700 KB**, switch; otherwise keep 224 and record the number.
- **Measured, 2026-10-30, through the bridge with one 768-px JPEG of `nave.jpg`:**

| export | host | cold call (server `elapsed_us`) | repeat | reply |
|---|---|---|---|---|
| 224 (onnx-community, fp32) | development host, 4 intra-op threads | **56 242 µs** | 547 µs | 301 056 B |
| 224 (onnx-community, int8) | deployed sandbox (`cli.emb.is`) | **93 041 µs** | 1 802 µs | 301 056 B |
| 336 (kornia, fp32) | development host, 4 intra-op threads | **372 956 µs** | 862 µs | **677 376 B** |

- **Decision: keep 224.** The reply passes easily (677 376 B ≤ 700 KB), but the latency does not survive the host correction: the same 224 export costs **1.65×** more on the deployed sandbox than on the development host (93 041 against 56 242 µs), and the 336 export's measured 372 956 µs on the development host carries to roughly **615 000 µs** — about 1.5× the 400 ms gate. The 336 export was never run on the deployed host itself, because pointing the sandbox at it is a deploy, and until this change ships the published page declares 196 rows and 301 056 B; a deployed 336 would falsify the live page's own figures for the duration of the test. The finer grid therefore stays out, and the numbers above are the reason.
- A swap would restate every figure the page prints (301 056 B → 677 376 B per grid, 196 → 441 rows, 14 → 21 cells across), which is why it is one task with its own verification rather than a tweak.

### D27. The library is Unsplash, and the licence path does not change

> **Amended by D36 and re-curated a second time.** Depth is the only selection gate; the same-place pair went with the correspondence figure. The library was then re-harvested for *kinds* of depth rather than more of the same scene, and the six below replaced the canyon-heavy set.

The library ships six **Unsplash** photographs, all arriving through Wikimedia Commons' Unsplash import, which is **CC0**: the Commons page is the licence of record, the Unsplash page is the source of the photograph, and the tree ships the mirror rather than a hotlink or a stock-site licence. The licence path does not change — it is the same `extmetadata`-read, CC0-or-public-domain rule the plate already had — and the photographer, handle, Unsplash page and Commons mirror are recorded per file in `CREDITS.md`.

- **Re-curated on the depth it carries, one kind per tile.** 352 candidates were searched out of Commons' Unsplash import across ten subject families, bounded to 768 px exactly as the page bounds them, and run through `EMB.IMG depth BLOB OUTPUTS depth`; six were kept, and the gate was the plate's own: spread ≥ 0.60, both deciles ≥ 0.01, gradient ≥ 0.006. The six that survived are six *different* structures rather than six landscapes:
  **perspective convergence** (`pier-corridor`, `forest-path`, `spiral-stairs`), **layered geology** (`canyon-layers`), **occlusion through a frame** (`sea-cave`, `rail-tunnel`).
- **Measured on the installed files by `website/tools/depth-gate.py`** — the committed tool that bounds a file exactly as the page does, calls `EMB.IMG depth BLOB OUTPUTS depth`, and applies the gate, so the table below can be reproduced with `just website-depth-gate`. Through the bridge on the development host, 2026-10-30; columns are `spread / near <0.10 / far >0.90 / gradient` and the reply's own `min / median / max`:

| file | scene | size | spread | near | far | gradient | min / median / max |
|---|---|---|---|---|---|---|---|
| `pier-corridor.jpg` | a wooden pier walkway at sunset | 768×481 | 0.849 | 0.104 | 0.020 | 0.0165 | 0.000 / 2.223 / 4.146 |
| `canyon-layers.jpg` | Canyon de Chelly, layered walls | 768×498 | 0.847 | 0.362 | 0.045 | 0.0070 | 0.000 / 0.724 / 3.370 |
| `sea-cave.jpg` | a rock arch over a turquoise bay | 768×509 | 0.889 | 0.259 | 0.041 | 0.0079 | 0.000 / 1.779 / 3.776 |
| `forest-path.jpg` | a gravel path between pines | 768×512 | 0.687 | 0.011 | 0.028 | 0.0149 | 0.000 / 1.511 / 2.977 |
| `spiral-stairs.jpg` | a stone spiral staircase, from above | 768×576 | 0.816 | 0.062 | 0.050 | 0.0067 | 0.278 / 1.928 / 3.415 |
| `rail-tunnel.jpg` | rails leaving a dark tunnel | 513×768 | 0.950 | 0.203 | 0.127 | 0.0076 | 0.000 / 1.733 / 5.913 |

- **The strip opens on `pier-corridor`.** It is the one whose relief reads in three planes at once — the near planks, the row of posts and the far end of the walkway — and its reflective floor gives the shading something to bend, so the 12.6° reveal has parallax from the first frame. `rail-tunnel` is the deepest map (spread 0.950) and the one that shows the cut best, but it is a dark frame and a weaker opener.
- **What the old set was.** `church-aisle`, `bridge-fog`, `snow-tunnel`, `pier-fog`, `canyon-rim` and `canyon-clouds` are deleted from the plate: two of them were the same canyon, and the rest were landscapes. Every one of their successors passes the same gate the old set did, with the same licence path.
- **Rejected:** keeping the two canyon frames for a "same place" pair (that figure is gone, and two canyons is one kind of depth); dunes (six candidates measured, none passed the gradient gate — a sand field is smooth at the map's cell size); night street scenes (they pass the gate and fail the relief: the shading has nothing to carry); a direct Unsplash import (it would put the Unsplash License in the tree beside the site's CC0 rule for no gain, since the mirror is the same bytes under CC0); more than six (the strip is sized for six, and each extra picture is a cold call on first visit).

### D28. The token strip is a raster, not a chart

> **Removed by D36.** There are no token rows to draw.

The features were explained by sentences ("196 rows of 384 numbers"). The figure draws the reply: a **384 × 196 raster**, one column per dimension, one row per patch in grid order, on a diverging ramp (`--accent` positive, ink negative, magnitude as the ramp).

- **Per row, not raw.** The first render drew the raw components and read as beige noise: a patch row's values sit close to the picture's own offset (the same reason the matrix's off-diagonal median is 0.60–0.90), so a linear ramp hides the response. The shipped raster z-scores **each row over its own 384 dimensions** before the ramp, which is what makes a patch's *own* pattern visible, and prints that row's raw range beside it so the normalisation is disclosed. Rendered and checked at 768 × 392 on a candidate's 196 × 384 reply (the torii corridor, which is not shipped): patches band coherently and adjacent patches agree, which is what the reading should show.
- **Drawn once, revealed cheaply.** The raster is built into one `ImageData` per sample (384 × 196 = 75 264 pixels) and the animation copies a growing slice into the visible canvas — no per-frame allocation, and the same sample produces byte-identical pixels on two draws, which is the invariant that lets a screenshot be evidence.
- **The scan is the explanation.** One patch every 60 ms (12 s per pass). The row under the scan is drawn as a 384-value sparkline, and the patch it belongs to pulses on the plan beside the stage, so the vector and the pixel are read together. Hover pauses, click or arrow keys pin that patch as the probe, and the figure owns a play/pause button because a 12-second loop beside other content needs one.
- **Numbers printed:** the patch under the scan, its 384 values' range, and the grid shape read from `EMB.INFO`.
- **Reduced motion:** the centre patch's row is drawn once, no scan, and the readout states its range.
- **Rejected:** an SVG of 75 264 rects (slower to build than the raster and no sharper); a per-frame gradient redraw (allocation in the loop); animating the photograph's own pixels (the raster is the evidence, the photograph is already the stage).

### D29. The similarity matrix prints its own chance level

> **Retired as a figure.** The matrix is deleted by the rework in D33–D35: the field prints the same floor, the strip prints the same range, and the figure was the third drawing of one cosine in the place the reader asked for something other than similarity. The measurement below is kept because the floor it established is still printed.

A cosine of 0.8 means nothing without the number it is being compared to, and this picture's chance level is measurable. The figure is the **196 × 196 matrix** of `cos(aᵢ, aⱼ)`, computed once per sample (38 416 dot products of 384, about 30 ms), drawn as one heat map.

- **The diagonal is the proof the server normalised.** It reads 1.000 by construction; a reader who doubts the reply can see it.
- **The off-diagonal median is this picture's own floor**, printed beside the figure — and it is high: measured 0.599–0.903 across the 657-candidate pool, and **0.664 (church aisle), 0.690 (canyon rim), 0.748 (foggy pier), 0.792 (canyon clouds), 0.803 (snow tunnel), 0.826 (bridge in fog)** on the shipped six. Dense-feature cosines are offset upward within one scene, so an absolute 0.8 means little until the floor is named; the figure prints both, and "alike" becomes a comparison against this picture's own baseline instead of an adjective.
- **The ramp is per picture.** The 5th–99th percentile of the off-diagonal is mapped to the ramp and the diagonal is drawn in ink, or a repetitive scene saturates the whole matrix to one colour. Rendered and checked on that same candidate: the block structure and the diagonal both read, where a linear 0–1 ramp produced a uniform orange square.
- **The scan links the matrix to the photograph.** A 6-second row sweep; the row's patch and its top five matches are ringed on the plan and on the picture. Clicking a cell pins the row's patch; the readout names both patches and the cosine.
- **Engineering:** one `ImageData` for the matrix (196 × 196 = 38 416 pixels), the row sweep drawn as a 1-pixel overlay layer rather than a re-raster, and the cell readout derived from a hit test on the canvas box (no per-cell DOM).
- **Reduced motion:** the current probe's row is drawn, no sweep.

### D30. The correspondence field is the claim, and the ladder is its caption

> **Promoted to the first figure** by the council's rework (D33–D35). The property that earned it: on an unrelated pair the field draws nothing and says so (`no cell clears the floor, so no arrows are drawn`), so the figure can fail on screen as well as succeed.

This is the figure that answers "what are these features *for*" with arithmetic the reader can check: for every patch of A, the argmax over B's rows — a dense correspondence with no decoder, no head and no labels.

- **Three numbers, always printed:** the median best cosine, the picture's **own within-picture floor** (its off-diagonal median), and the share of B's cells that **nothing chose**. Measured on the committed files: median **0.798** one way and **0.784** the other, the asking picture's floor **0.690**, and **81% of the second canyon's cells were chosen by nothing** — the field collapses onto a few patches, and saying so is the honest reading. On the page the same pair reads **0.788 / 0.800** and **79–80%** unchosen, because the browser re-encodes the picture before it is sent; the readout is the live number and the copy quotes the committed files. The share-above-a-threshold statistic was rejected after measurement: on the same-place pair it was 1.00 (every cell cleared 0.60), which discriminates nothing.
- **The ladder is the library's own measurement:** 0.798 on the same place (the canyon pair), 0.492 on the same composition in a different place (aisle against pier), 0.371–0.556 on unrelated pictures. The composition pair sits *inside* the unrelated band, which is the finding: composition alone does not separate on this checkpoint, and the caption says so. The copy's conclusion is narrower than "structure, not identity" and true: **the field is dense where the place is the same.**
- **It is a direction, not an equivalence.** A checkbox asks for the reverse field (B → A); both medians are printed so the asymmetry is visible rather than asserted.
- **Rendered and corrected before it was specified.** The first render tinted A's cells by the raw cosine and drew every arrow: A saturated into one orange field and the arrows became a wash. The shipped rule: **both grids are normalised by the pair's own 5th–99th percentile**, arrows are drawn **only where the cosine clears the asking picture's floor** (so the picture's structure shows through the field), their opacity is the normalised score and their colour climbs from rule to accent, and a click opens B at that cell.
- **Cost:** 196 × 196 dot products of 384 per pair, against grids the strip already holds (the library lookups), so the figure issues no call. The arrows animate over 2.4 s, drawn in sequence; 49 lines (every second cell) is the ceiling, fewer when the floor cuts them.
- **Rejected:** a warp of B toward A (a coarse piecewise remap that would read as a morph and imply a fidelity the 14 × 14 grid cannot carry); drawing all 196 arrows (a hairball; 49 carry the same information); a learned matcher (that is a different model, and the point is what the served one can do alone).

### D31. The figures' engineering contract

- **Canvas 2D, one dependency-free module.** No WebGL for the figures: the stage already owns a context, and three more would fight for the same GPU on a phone. 2D canvases are drawn at `devicePixelRatio` and rendered `pixelated`.
- **Nothing animates off-screen.** One `IntersectionObserver` per figure starts and stops its loop; a figure that has played once does not replay without a press.
- **Every control is native.** Play/pause is a `<button>` with `aria-pressed`; the second picture is a `<select>` with a `<label>`; the reverse field is a checkbox; each canvas is `role="img"` with an `aria-label` that carries the numbers, so the animation is never the only way to read the figure.
- **No new detector findings.** The figures sit inside the plate's existing type ladder: captions at the committed 12 px mono in `--muted`, no new wide tracking, no `transition: all`, no padding below the shell's scale, and the interactive targets are 44 px. Measured with `impeccable detect website --json` (v4.0.0) after the rework: **211 findings across the tree, 8 on this plate** — `cramped-padding` ×5, `clipped-overflow-container` ×2 and `wide-tracking` ×1, all shared with the shell. The plate's own `tiny-text` and `low-contrast` notes are gone (11 → 8); an extra `em-dash-overuse` advisory appeared with the first draft of the copy and was cleared by thinning it. The trace and the figure captions must not add a ninth.
- **Reduced motion, settled in one frame.** `motionAllowed()` gates every loop exactly as the reveal does; the figures then draw their selected state, and the measured check confirmed the strip's scan does not advance across two seconds while the field is fully drawn.
- **Measured on the page (2026-10-30):** the strip raster builds in **6 ms** for one 196 × 384 reply (the page's own loops, timed on the reply the bridge returned) and the mosaic is 196 `drawImage` calls from the same reply; the page holds **61 frames/s** with the stage orbiting and all three canvases live; a figure interaction issued **0 commands**; on a 390-px viewport the figure row collapses to one 350-px column with no horizontal overflow, the field canvas is 350 × 139, and the trace is DOM text at 12 px rather than the removed SVG's 5.3 px labels.
- **A figure cannot lie about the reply.** All three read `S.patches` (or a library entry's rows) and the declared shape; if the grid is absent the figure states why and draws nothing.

### D32. The name is sentence case, and the copy follows the figures

- **The perception**, not "The Perception": a plate title, like "The medium" and "The image". The rename touches the page's `<title>` and metadata (through `website/tools/pages.json` and `stamp-pages.py`), the head's `VII. The perception`, the gallery's index entry, the gallery's list of plates, the spec, and the surface brief. The route, the rig selector and the plate number do not move.
- **The copy stops arguing and starts pointing.** `WHAT YOU ARE LOOKING AT` names the four depth mechanisms the library covers; `WHAT JUST HAPPENED` introduces the three figures with one sentence each and their live readouts; `WHY IT MATTERS` states the ladder and what it means (structure, not identity); `THE EXACT COMMANDS` keeps its specimen with the new filenames. The text budget above the stage is unchanged: the figures are below it.
- **Every number is printed before it is written.** The copy's constants are the library's measured table (D27), the ladder (D30), the reply's shapes (`EMB.INFO`), and the crop's measured hit rate. A sentence that cannot be traced to one of those does not ship.

### D33. The grouping gets a figure, and the figure prints the baseline that embarrasses it

> **Removed by D36.** The mosaic, its ruler and its medoid arithmetic go; the measurement that killed it stays in D35.

The council's measured finding: **a content-blind 8-horizontal-band cut of the same 14 × 14 grid scores 0.731 4-neighbour coherence**, while the page's seeded k-means scores 0.670 (aisle), 0.690 (bridge), 0.692 (pier), 0.681 (canyon clouds), 0.717 (tunnel) and 0.766 (canyon rim) — **better than the ruler on one picture of six**. Measured with the page's own seed over the page's xorshift k-means; the earlier "≈ 1/8 by chance" null was the wrong null, because a random-label baseline is trivially beaten by any spatially smooth quantity.

So the grouping is not the headline — it is a figure that shows what the model groups **and** what a ruler would do with the same lattice:

- **The figure is the mosaic:** every patch replaced by the crop of its group's centre patch (`medoid`), cut from the same photograph and repeated — a **lookup, not a reconstruction**, and the caption says so. One offscreen canvas, 196 `drawImage` calls per sample, byte-identical on a second draw, no call.
- **The ruler is a toggle:** a native checkbox swaps the mosaic for the same grid cut into eight horizontal bands. The two coherence numbers print side by side (k-means vs ruler), and so does the count of four-connected pieces per group (1–7), which is the anti-class tell: a category does not arrive in five pieces.
- **Two more numbers, both unflattering and therefore printed:** the closest two groups' centre-to-centre cosine (median 0.643–0.832, max 0.904–0.958 — a detector would not report that two of its categories are 96% the same) and each group's member count (as low as 1 of 196).
- **The medoid is not a prototype.** Measured: the medoid beats the second-best member by 0.001–0.005, so "the model chose this exemplar" would be false; the caption says *nearest its group's own mean*, and the figure claims no more.
- **The click is the probe's.** Clicking a mosaic cell pins that patch through the existing probe; the readout gains `part k · n patches · centre #j`. No sixth control, no new state.
- **The legend stays.** The eight swatches key the relief's vertex colours in the `parts` reading, which a mosaic of crops cannot do; the mosaic is self-keying because a cell *is* its group's crop.

### D34. The pipeline graph becomes the run's own trace

The static SVG printed typed byte counts — the one figure on the page exempt from "a figure cannot lie about the reply". It also scaled its 11-px labels to **5.3 px** and its nodes to 31–33 px at 350 px, so making its nodes targets would break the 44-px rule.

- **The figure is the trace:** the request's bounded JPEG bytes (`boundedImage.bytes.length`), each head's measured time and cached flag (`elapsedFeatures`, `elapsedDepth`), and one bar per reply **scaled by its actual bytes** — `embedding` 1 536 B, `patches` 301 056 B, `depth` 283 024 B, from `values.*.count × 4`, with the declared shape from `EMB.INFO`. The scale is the point: the pooled vector is half a percent of the grid; "one pass, two products" becomes a length.
- **The order is drawn honestly:** the run awaits features, then depth; the trace lays the two heads in sequence, not as a parallel fork.
- **The reader's action:** each reply is a native 44-px `<button>` whose label is its own numbers; pressing it focuses the figure that reads it (patches → the strip, pooled → the strip's unpinned ranking, depth → the stage). No command, no loop, nothing to gate under reduced motion.
- **Without scripting the old schematic still explains the fork** (it stays in the prose beside the nine-step fold); the trace only appears when a reply exists, which is the page's existing pattern for every derived figure.

### D35. What was explored and rejected, with the number that killed it

Every candidate below was measured on the six shipped replies before it was refused; the numbers are what the record keeps, not the taste.

| candidate | claim it would print | the measured kill |
|---|---|---|
| **A. direction search** `normalize(X − Y)` | "more of this, less of that" | the query is a difference of two similarity columns: X ranks 2nd–7th, Y ranks 191–195; **0% of 400 random queries put any cell above the picture's floor (0.66–0.83)**; top score 0.31–0.42; top-20 overlaps plain similarity-to-X by 50–62%; moving X inside its own cluster leaves the map unchanged (r 0.98–1.00) |
| **B. typicality map** `cos(row, pooled)` | "how much each patch is the picture" | **88–97% of its variance is between the existing eight clusters**; the within-cluster residual has neighbour r 0.21–0.41; its correlation with the grid's vertical axis flips sign between pictures (+0.68 aisle, −0.50 tunnel). It is the parts reading with a gradient |
| **C. eight prototype tiles** | "eight parts, one exemplar each" | the medoid beats the second-best member by **0.001–0.005** — there is no distinguished member; the tiles also invite the object-detector reading the risk list forbids |
| **C′. the mosaic** | "the picture rebuilt from its own groups" | not a prototype claim and it renders, but a **content-blind ruler scores 0.731** against 0.670–0.766, so it ships as a *test* (D33), not as the headline |
| **D. exemplar-guided depth** | "features sharpen the relief" | **the cosine weighting is a no-op**: against a photo-only guided filter, mean absolute rank change 0.0006–0.0119, only 0–3.8% of pixels move more than 5%; the photo alone does the work, and the 14 × 14 guidance is coarser than the 19 × 19 depth tokens it would filter |
| **E. feature-space map** | "a semantic map" | PC1+2 explain 61–73%, but **PC1 is the row axis** (ρ 0.28–0.95), so the scatter draws the photograph's layout coloured by a clustering of the same space — the reason PCA was deleted in D19, now measured |
| **F. hierarchy** | "global vs local" | undefined: the pooled direction is 25–40% of squared norm and the clusters 88–97% of typicality, so it is B plus C with no new baseline |

### D36. The second model is removed, and the measurements are the reason

The plate keeps one model. DINOv3, its patch grid and every figure built on it are deleted from the page, from the sandbox's model set and from the bridge's image surface.

- **Why, in numbers the page itself printed.** Every remaining use was one cosine in another coat (D35): the direction search never cleared the picture's floor (**0%** of 400 queries), the grouping was beaten by a content-blind ruler on five pictures of six (**0.731** against 0.670–0.766), typicality was **88–97%** explained by the clusters, the correspondence field was silent on **79–81%** of the second picture's cells and its "same composition" band (0.492) sat inside the unrelated range (0.371–0.556), and the eight prototype tiles differed from their own second member by **0.001–0.005**. A feature surface whose every demonstration reduces to the same dot product is not a demonstration.
- **What is deleted.** Page: the `parts` reading and `clusterPatches`/`clusterCoherence`, the probe/region/`isolate`/`drawBox`, the crop and its grading, the six-picture answer strip, the token strip, the grouping mosaic and its ruler, the correspondence field, `paintSim`, `selfFloor`, the `aPart`/`aPatch` attributes and `uParts`/`uSim`, and the DINOv3 lane of the trace. Sandbox: the `dinov3` entry (~90 MB resident, 22 MB on disk) and its image session. Bridge: `dinov3` from `imageModels`, from the surface string and from the refusal, leaving one image model; the table tests lose their `dinov3` rows and gain a refusal that names `depth`.
- **What is kept on the server.** The `tokens` selector and its tests: it is a registry capability, not a plate's, and removing it would spend the change's risk on a different surface. Nothing in `internal/` changes.
- **What is kept on the page.** The stage, the reveal and its clock, the surface and normals readings, the cut, the exaggeration, the camera clamps, the plan canvas, the six photographs and their credits, the prose sections and the disclosures, the sentence-case name.
- **Cost and consequence.** One call per sample instead of two, one image model in the surface, ~90 MB less resident, and one question the reader can restate. The plate loses "two models against each other" and the gallery line changes with it.
- **Rejected:** keeping DINOv3 behind a disclosure as "the other half" (a second vocabulary with no question), keeping only the crop (it is the same cosine with a box around it), keeping the grouping as a negative result (the plate would teach what a ruler already does).

### D37. The map is a figure, at the picture's own size

The reply is 266 × 266 of relative depth, and the plate had never shown it as itself. It becomes the first figure under the stage, replacing the plan's similarity tint:

- **What it draws:** the map as a raster on the same aspect as the photograph beside it, one cell per map pixel, on a ramp whose limits are the map's own p5/p95 (printed), with an `raw` / `rank-equalised` native toggle. The equalised view is the transform the relief uses (D17), so the toggle explains why the geometry needs it: raw, most of a narrow street sits at the near end; equalised, the deciles are even.
- **What it prints:** `rows × dim`, min/median/max, the p5–p95 band, and the exact float under the pointer. The value is read from the reply, not interpolated for display.
- **What it keeps:** the canvas is still `role="img"` with a label carrying the shape and range (the accessible, WebGL-free figure), and it is drawn from the reply on every sample (`D4`).
- **Cost:** one 266² `ImageData` per sample (70 756 pixels) plus its rank twin, rebuilt only when the sample changes; the canvas is 532 px so the cells stay square at any display size.
- **Measured on the six shipped pictures** (development host, through the bridge, `EMB.IMG depth BLOB OUTPUTS depth`): the reply is 70 756 values and 283 024 B in every case, and each map's own range is `min / median / max`:
  `church-aisle` 0.000 / 0.969 / 4.268 with p5–p95 0.020–4.045; `bridge-fog` 0.000 / 0.180 / 1.833, 0.000–1.685; `snow-tunnel` 0.000 / 2.532 / 4.387, 0.000–3.570; `pier-fog` 0.000 / 0.064 / 4.456, 0.000–3.935; `canyon-rim` 0.000 / 0.935 / 4.952, 0.000–4.519; `canyon-clouds` 0.000 / 0.571 / 4.331, 0.000–3.643.
- **Hover gate, checked live:** a pointer at 37% across and 61% down the canyon map read `pointer 0.311 at row 162, column 98 · depth[43190]`, and 162 × 266 + 98 = 43 190, so the printed value is the reply's element at the printed index.
- **The ramp is two tokens,** `--rule` far and `--accent-ink` near, so the figure cannot drift from the palette; it is drawn from the reply on every sample and never from a stored image.

### D38. The profile turns the relief into a measurand

A drag across the photograph — or the figure's own three native inputs, `from`, `to` and `row`, so the line is reachable without going back to the stage — defines a scanline, and the page samples the map along it with the **same bilinear reader and the same rank equalisation the vertex shader displaces by** (`depthHeight`), then draws the profile in display units. The chart's window is the line's own 2nd–98th percentile, because one outlier cell would otherwise flatten every other point into a straight run.

- **What it prints:** the near and far values at the endpoints, the mean absolute slope along the line, and the span in frame-heights. The endpoints are marked on the photograph and on the relief, so a reader can see which part of the surface the graph came from.
- **Why it is honest:** the profile cannot disagree with the geometry, because it is the geometry's own function; the gate samples 100 points and asserts equality within 1e-6.
- **What it is not:** metres. The y axis is in frame-heights of display depth, and the copy says so, because the model's output is relative.
- **Cost:** 266 bilinear samples per move, no call, no allocation beyond the chart's path.
- **The reply's own values at the two ends are printed too** (`the map there reads 3.392 and 1.067` on the canyon at v 0.50), so a reader can check the chart's field against the raw map beside it. The profile's values are the blurred, rank-equalised field the shader displaces by; the printed pair is the reply's own number at the same pixel.
- **Measured on the six** (default line, v 0.50, x 0.08–0.92): mean absolute slope **0.063–0.582 per frame-width** and span **0.029–0.176 frame-heights** — `church-aisle` 0.271 / 0.176, `bridge-fog` 0.582 / 0.115, `snow-tunnel` 0.336 / 0.140, `pier-fog` 0.360 / 0.059, `canyon-rim` 0.220 / 0.126, `canyon-clouds` 0.063 / 0.029.

### D39. The focus is the depth map as a lens, with its failures left in

The third reading is photographic: the map as a depth of field. Two native inputs — `focus` (a percentile of the map, never metres) and `aperture` (the width of the sharp band) — blur the photograph by `|d̂ − focus|`, and a sharpness readout measures the result.

- **How it renders:** six quantised bands, and every pixel of the figure takes its 4 bytes from the one pre-blurred copy its band's level names — six `ctx.filter = blur(r)` levels, each read back once per sample, then one selection pass over the `ImageData`. Canvas compositing was the first approach and was dropped: masked band draws left seams where two levels met, and the seams (not the map) dominated the measurement.
- **What it prints:** the focus percentile, the aperture as a percentile band, and the **detail each region keeps** — the sharp bands' Laplacian variance over their own variance in the untouched photograph, and the same figure for the defocused bands — measured on a 256-px copy with a two-pixel rim removed at every band edge. The content cancels because both sides are divided by themselves, which is why a foggy picture and a canyon can be compared at all. The printed **sharpness ratio** is inside ÷ outside; the gate requires ≥ 3 on every shipped picture.
- **Measured on the six** (focus 35th percentile, aperture 18 points): detail kept **100% inside** against **0–1% outside**, a ratio of **67×–680×**; a slider change renders in **10–21 ms** (the sample's six blur levels and its band map are built once, at first draw, in 90–120 ms).
- **What it shows honestly:** the map's failures stay visible. A window, a thin branch, the tunnel's mouth: the blur follows the map's mistake, and the caption says the figure is the map, not a lens.
- **Cost:** one selection pass over a 640×428 `ImageData` plus one 256-px readback per change — **10–21 ms measured**, inside the **≤ 40 ms** budget, no call.
- **Rejected:** a per-pixel blur at 768² (a full-resolution separable kernel on the main thread for an effect six bands already carry), a "bokeh" glyph pass (a second shader for a photographic effect the bands deliver), and hiding the failures (they are the point).

### D40. What the plate says, and what it refuses to say

- **One model, one question:** how a machine sees the distance of things in one photograph, and what that map is good for.
- **The limits, unchanged and now load-bearing:** the depth is monocular, relative and approximate; one face, no backside; no metres; the relief's cut leaves holes; the profile is in frame-heights; the focus is the map used as a lens, not a lens.
- **The copy drops** every sentence about patches, vectors, cosines, clusters, crops, correspondence and "two models"; the definitions list keeps the depth limits and the camera notes.
- **The gallery line and the descriptor** name one depth model, the map, the profile and the focus, and nothing about search.
- **The text budget** is unchanged: title, one sentence, the stage, the filmstrip, the state/reading controls and the readout above the fold; the map, the profile and the focus are figures below it, with their own captions.

### D41. The figures are operable where they are, and they cost nothing

A figure that needs the stage to be used is a figure the reader scrolls past. Each of the three is therefore self-contained:

- **The map** draws from the reply and owns its `raw` / `rank-equalised` toggle and its hover readout; nothing above has to be touched. The strip opens on `pier-corridor` (D27), the picture whose relief separates three planes at once.
- **The profile** owns `from`, `to` and `row` as native ranges inside its own figure, so the line can be placed by keyboard or pointer without returning to the still. The drag on the photograph writes the same state — one profile, two routes, never two definitions.
- **The focus** owns its `focus` and `aperture` ranges. No figure reads the stage's view or reading state, so scrolling straight to them and interacting yields the same result.

Measured on the local stack (1440×900, `just website-dev`):

| what | measured |
|---|---|
| bridge calls on a fresh load | **2** — one `EMB.INFO depth` (the declared shape, read once) and one `EMB.IMG depth BLOB OUTPUTS depth` |
| calls issued by the map toggle, three profile inputs, both focus sliders | **0** |
| frame rate with the stage orbiting and all three figures live | **61 fps** (the compositor's cap) |
| phone at 390×844 | no horizontal overflow, figures stack to one column, controls wrap, canvases 100% wide |
| `website-ink` on `demos/perception.html` | `PASS — no text ink outside the viewport at any of 24 widths` |
| `impeccable detect website --json` | **211 findings tree-wide, 8 on this plate** — `cramped-padding` ×5 (the site's `section.block` inset), `clipped-overflow-container` ×2 (`html`/`body`), `wide-tracking` ×1 (the masthead tagline): all shared with the shell, and none added by the figures |

### D42. The review's fixes, and why each one earned its lines

A senior review of the delivered plate found ten things; all ten were fixed. The ones that touched behaviour:

- **A generation counter on the run.** `sample()` and the upload handler bump one module counter, and `run()` returns as soon as it is stale after any awaited step. Without it, clicking two tiles quickly let the slower reply win and the page showed photograph A under tile B.
- **The focus figure rewrites its buffers instead of allocating them.** The paint target (`ImageData`, 1.09 MB at 640 × 428) and the luminance buffer (256-w pixel copy) live on the sample's cache, and the two slider inputs coalesce to one render per `requestAnimationFrame`. Measured: 80 synthetic `input` events went from 1 032 ms of main-thread work to **11 ms**.
- **`ctx.filter` is feature-detected.** Canvas blur does not exist in Safari before 18; there every band would have been the sharp photograph and the readout would have printed a ratio of 1 under a caption claiming a depth of field. The figure now states the limit and hides the canvas instead of drawing a lie.
- **The upload's object URL is released on the next upload** (the code this replaced did that; the rewrite had dropped it). Measured with counters patched: two uploads create four URLs and revoke three, so exactly one is live.
- **The profile's press needs six pixels of travel,** the same rule the stage uses, so a phone scroll that starts on the still no longer rewrites the profile; and a plain tap leaves it alone.
- **The phone gets the tier it can resolve.** `buildReconstruction` caps the density from the stage's CSS width (step 2 under 520 px), which took a 390-px stage from 390 912 vertices to **92 544** — the phone was drawing four times the geometry its screen can show, and blocking the main thread to build it.
- **The map canvas answers its keys.** It is a tab stop because the trace focuses it; arrows now walk the readout cell and `Home` returns to the middle, so the stop is not dead.
- **The two profile end inputs clamp instead of swapping,** so dragging `from` past `to` no longer reverses the thumb under the pointer.
- **The depth gate is committed.** `website/tools/depth-gate.py` (+ `just website-depth-gate`) bounds a file exactly as the page does, calls `EMB.IMG depth BLOB OUTPUTS depth`, and prints the four gate columns. Re-run over the shipped six it reproduces the table in D27, so the library can be re-curated without re-inventing the measurement.
- **Dead code deleted:** an unused `INK` token and `samplesEl` handle, the `data-profile-overlay` and `data-strip-hint` hooks nothing read, and the trace handler's unreachable `mapCanvas` branch.

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
- **A clustering that reads as an object detector** → the copy names the clusters as the model's own distances with no labels and no head, the readout never prints a class, and the reading is offered beside the photograph rather than instead of it.
- **A crop that "finds" itself by colour** → the query is the model's returned rows, not pixels, and the page prints the measured hit rate and the rank instead of implying a certainty; a box smaller than the measured size is outside the claim.
- **A library that still contains no matches** → the matchability gate is measured before any photograph is committed, and the strip's pooled ranking prints its cosine, so an unrelated library reads as unrelated rather than as a failed search.
- **A finer grid that costs more than it shows** → the 336 export ships only if its measured cold call and reply size pass D26's thresholds; otherwise the page keeps 196 and says so.

- **A similarity matrix read as a search result** → the off-diagonal median is printed as the picture's own chance level, and the caption says the matrix is one patch against every patch of the same picture.
- **A dense correspondence read as identity** → the ladder is printed (same place 0.798, same composition 0.492, unrelated 0.371–0.556), the caption names which pair is which, and the copy says the field is dense where the place is the same and that composition alone does not lift a score.
- **Three animated figures costing the frame budget** → each raster is built once per sample, each loop stops off-screen and on reduced motion, the arrows animate as 49 lines and not 196, and the figure gate measures the stage's frame budget with all three visible.
- **A figure that looks better than its reply** → every canvas carries `role="img"` and an `aria-label` with the numbers, and each readout prints the median, the share and the silence rather than a rounded impression.
- **A loop nobody can stop** → every figure owns a native play/pause button, plays once on entry, and never replays without a press.
- **Unsplash licence drift** → the tree never links a stock site: the committed bytes are the Commons mirror, the licence is read from its `extmetadata` as CC0, and the Unsplash page is credited as the source of the photograph.
- **A figure disagreeing with the stage** → the figures read the same `S.patches`, the same library entries and the same probe, so a click in either pins the same patch and a sample switch redraws both.

## Migration Plan

1. Ship the server's `map` selector and the depth model together with the bridge's second model name; deploy the sandbox by hand (`just sandbox-deploy`).
2. Publish the page after the surface answers; a refused command leaves the plate in its stated unavailable state.
3. Rollback is removing the page's descriptor entry and the bridge's depth name; the server's selector and the model are inert without them.
