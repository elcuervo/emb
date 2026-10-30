---
version: 1
slug: "website-demos-perception-html"
primary_target: "website/demos/perception.html"
related_targets: ["website/demos/index.html"]
---

# Surface brief — The perception

## Scope and visitor mode

**Read and play.** One plate at `website/demos/perception.html` for `emb`, the
gallery's fifteenth and plate VII, named **The perception** in its title, its
head, the gallery's index entry and the gallery's list of plates. The visitor
arrives from the gallery or a search; the plate answers *what one depth map is
worth once it is geometry*: one photograph goes to Depth Anything V2 Small
(Apache-2.0) and comes back as a 266 × 266 relative map of 70 756 values, and
every figure on the page is that reply plus the pixels the browser sent. The
still sits beside its reconstruction, and the reply is read four ways: the lit
surface you can turn and cut, **the map drawn as itself**, **the profile along a
line you drag**, and **the map used as a depth of field**. The plate is an
extension of the gallery's committed world, not a new one: the palette, type
ladder, dark-plate apparatus and caption discipline come from `DESIGN.md`, and
the sample's own aspect sets the stage's frame.

## The visitor's job

Pick a photograph from the strip — or drop one in through `adjust` — and the
plate reveals it by itself: the still hands over to its own pixels, the surface
lifts out of the depth map over three seconds, and the light catches it as the
camera turns 12.6° inside a clamp that never shows a backside. Turn it, wheel
into it, choose `surface` or `normals`, set the cut or the exaggeration. Then
read the map below: every cell is a value the server sent, one toggle away from
the rank transform the geometry uses; drag a line and the profile prints the
depth along it in the shader's own units; slide the focus and the photograph
refocuses by depth, with the detail each region keeps printed beside it. Leave
knowing that the depth is monocular and relative, that the profile is in
frame-heights and not metres, that the focus is the map used as a lens rather
than a lens, and that the places the map is wrong stay wrong on screen.

## Constraints (inherited, non-negotiable)

- Inherit the world in `DESIGN.md` as it stands — the warm paper, `Public Sans`
  body, `Archivo` display. The plate adds no colour, font or dependency of its
  own: the stage is the gallery's dark block, its labels are the committed mono
  caption, and the one accent marks the pressed state, the reveal cue, the
  profile line and the map's near end. Its own material is six photographs in
  `website/demos/samples/parts/` — Unsplash originals shipped as their Wikimedia
  Commons CC0 mirrors, chosen on the depth their maps carry — and the vendored
  three.js module, loaded only when the stage first draws.
- **One call, one model.** The page reads the model's declared shape once
  (`EMB.INFO depth` → `select: map`, `rows: 266`, `dim: 266`) and refuses to draw
  when the reply's element count disagrees with it. Every printed shape, byte
  count and time is the reply's or the bridge's, never typed. The head's ledger
  (`THE MODEL · Depth Anything V2 Small · 266×266 MAP · 70 756 VALUES · 283 024 B
  · 1 CALL`) is filled from the reply.
- **Two states, two readings.** `photograph · relief` and `surface · normals`,
  plus exaggeration, cut, density and upload inside one `adjust` disclosure. No
  probe, no cluster reading, no crop, no cross-picture ranking: the plate asks
  one model one question.
- **Every figure is operable where it is.** The map's toggle, the profile's two
  ends and row, and the focus's two sliders live inside their own figures, so a
  reader who scrolled past the stage never has to go back up to use them. The
  drag on the still writes the same profile state, never a second one.
- **A figure cannot lie about the reply.** All three read the same map; the map
  figure's pointer value is `depth[row × dim + col]` exactly, the profile samples
  the shader's own bilinear reader and rank transform, and the focus's sharpness
  is measured, in the browser, on the canvas it just drew.

## What the surface carries

The head: the number and title, one sentence naming the model and the three
readings, the TL;DR, and the ledger. Then the stage block — the still and the
reconstruction side by side, the filmstrip, the state and reading controls, one
measured readout line, and the camera hint. Nothing prose-shaped sits above the
stage. Below it, in the gallery's own order: `WHAT YOU ARE LOOKING AT`,
`WHAT JUST HAPPENED` (with the three figures and the run's trace), `WHY IT
MATTERS`, and `THE EXACT COMMANDS`.

The figures: the map as one 266² raster with `raw` / `rank-equalised`; the
profile as a line chart in display units with its two ends marked on the still
and drawn on the relief; the focus as the photograph banded by depth with the
detail it keeps printed. Canvas 2D only — no WebGL below the stage.

## Accessibility and restraint

- The map canvas is the `role="img"` figure: with no WebGL or no module the plate
  says so and the map, the profile and the focus still draw from the same reply.
- Every control is native — radios, ranges, a checkbox, a file input — at the
  site's 44 px target, and the profile and the focus are fully operable by
  keyboard from inside their own figures.
- Reduced motion lands on the settled relief in one frame; no figure depends on
  an animation having played.
- The copy keeps its limits: monocular, relative, one face, no metres, the
  profile in frame-heights, the focus as the map used as a lens.

## Gates

1. **Map:** the pointer's value equals `depth[row × dim + col]`; the count equals
   `rows × dim`; the toggle redraws from the rank map the relief uses; no
   request on either.
2. **Profile:** 100 sampled points equal the shader's height function; the ends'
   readout matches the map; dragging or moving an input issues no request.
3. **Focus:** a change renders in ≤ 40 ms and issues no request; the sharp band's
   kept detail over the defocused bands' is ≥ 3 on every shipped picture.
4. **Removal:** no `dinov3`, `patches`, `cosine`, `cluster`, `crop` or
   `correspondence` string survives in the page; a fresh load issues one image
   command and one `EMB.INFO` read; the figures issue none.
5. **Site:** `node --check`, `website-pages-check` after a re-stamp,
   `website-presets-check`, `website-published`, `website-ink` at 24 widths, and
   `impeccable detect website --json` no worse than the plate's eight findings,
   all shared with the shell.
