---
version: 1
slug: "website-demos-perception-html"
primary_target: "website/demos/perception.html"
related_targets: ["website/demos/index.html"]
---

# Surface brief — the perception

## Scope and visitor mode

**Read and play.** One plate at `website/demos/perception.html` for `emb`, the
gallery's fifteenth and plate VII. The visitor arrives from the gallery or a
search; the plate answers *what a second model adds to a picture*: one
photograph goes to DINOv3 ViT-S/16, which returns a pooled vector and a
196 × 384 grid of patch features, and to Depth Anything V2 Small, which returns
a 266 × 266 relative depth map. From those two replies the page rebuilds the
photograph as a depth-shaded surface, reads it five ways, and probes a part to
rank it inside the picture and across the other five photographs. The plate is
an extension of the gallery's committed world, not a new one: the palette, type
ladder, dark-plate apparatus and caption discipline come from `DESIGN.md`, and
the sample's own aspect sets the stage's frame.

The gallery's entry for the plate is its author's and was not rewritten with the
rework. It still promises "one indexed surface, read five ways … with depth
cuts, a grown region and a probe that finds the same part in the other five
photographs", which the shipped plate still does; it does not name the
`photograph` state or `look-alikes`, which the plate's own head does.

## The visitor's job

Pick a photograph from the strip — or drop one in through `adjust` — and the
plate reveals it by itself: the still hands over to its own pixels, the surface
lifts slowly out of the depth map over three seconds, the light catches it as the
camera turns 12.6° inside a clamp that never shows a backside. Turn it, wheel into it, pick a reading, set the cut, isolate a
part; hover to ask a question and click to pin the answer. Then read the strip:
the other five photographs ordered by how much the pinned part looks like them,
each tile carrying its cosine and the cell it landed on, and a tile opened at
that cell follows the pin into that photograph. Leave knowing that depth is one
model's and relative, that the vectors are another model's and one per patch,
that a high cosine elsewhere is a similar patch and not the same object, and
that the region is grown from the probe rather than predicted.

## Constraints (inherited, non-negotiable)

- Inherit the world in `DESIGN.md` as it now stands — the warm paper, `Public
  Sans` body, `Archivo` display. The plate adds no colour, font or dependency of
  its own: the stage is the gallery's dark block, its labels are the committed
  mono caption, and the one accent marks the pressed state, the reveal cue, the
  probe and the cosine. Its own material is six photographs in
  `website/demos/samples/parts/` — four CC0 and two public domain, chosen by the
  depth their maps carry rather than by eye — and the vendored three.js module,
  loaded only when the stage first draws.
- Two calls, two graphs, one photograph. The page reads both models' declared
  shapes once (`EMB.INFO`) and refuses to draw when either reply's element count
  disagrees with the declaration; every printed shape, byte count and time is
  the reply's, never typed. The head's ledger (`THE TWO · DINOv3 ViT-S/16 · 196
  PATCHES · 384 DIMENSIONS · Depth Anything V2 · 266×266 DEPTH · 301 056 B · 2
  CALLS`) is filled from those replies.
- Three states, one reading control. `photograph · relief · look-alikes` and
  `surface · wireframe · normals · contours · cloud` are native radios drawn as
  the site's tab row, visible under the stage; every other control lives in one
  `adjust` disclosure — exaggeration, cut, density (three tiers), isolate,
  upload, probe column and probe row. Picking a reading other than `surface` on
  the photograph asks for the relief first, choosing the photograph resets the
  reading to `surface`, and the column and row sliders are the probe's keyboard
  route: a hover or a slider is a question, only a click pins.
- One surface, five readings. The index carries two triangles per accepted cell
  and the wireframe draws the same edges, so no reading can draw topology the
  surface does not have; a reading is a uniform and an index, never a rebuild.
  Cloud is a reading, not a slider: the mesh control, the surface⇄cloud slider,
  point size, the near/far slice and `all depths`, spacing, the guides, the
  `alike` and `region` sliders, the reset button, `back to the photograph` and
  the old multi-field HUD are deleted, not hidden.
- The strip is the answer, not a source list. With no patch pinned, the other
  five photographs are ordered by the pooled vector's cosine and each reads
  `whole picture`, while the photograph on the stage keeps the first row and
  shows its own best other cell. With a patch pinned, the plate grows the region
  from it, averages the member rows, normalizes once, and ranks each of the
  other five by the best dot product over that picture's rows, printing the cell
  and the cosine. Both are arithmetic over replies the page already holds: one
  lookup per other photograph, kept in memory for the visit, no index and no
  ranking call.
- The copy keeps four limits on the surface: the depth is monocular and
  relative, with no metres and one face; the normals are the map's own gradient,
  a convincing orientation rather than a recovered normal; a high cosine in
  another picture is a similar patch, not the same object; and the region is
  grown at fixed tolerances, so it can leak along a wall and stop at a shadow,
  and its box is the region's extent, not a detector's output. No class,
  category or object name appears anywhere in the figure.
- Accessibility is the plan and the keyboard, not a fallback. The plan canvas is
  the `role="img"` figure — the photograph underneath, the region washed over
  it, the probe ringed — with a label derived from the reply that names the
  probe's cell, the region's patch count, the best other cell and its cosine.
  The stage takes focus and answers arrows, `+ − 0`; the readout is
  `aria-live`; `prefers-reduced-motion` lands the reveal settled in one frame
  and stops the contour sweep; without WebGL the plate says the field is not
  drawn and the plan and the readout still read the same two replies.
- Read with JavaScript disabled: the head, the ledger and the four prose
  sections are complete and static, and the noscript line states that the
  interactive part needs scripting.
- No number is inferred from the picture. The readout reports the probe, the
  region's extent, the best other cell, the cosine, both declared shapes, the
  tier's vertex and triangle counts, the two measured times and whether either
  reply came from the cache.

## Direction contract

THESIS: The photograph is the plate and the model is the overlay. The frame is
the sample's own aspect and the still is the bytes the browser sent; every model
answer is a pass laid over that same image — the depth lifted into a surface,
its gradient as the light, its ordering as isolines, the patch cosine as a wash,
the probed part as a ring and a grown region. It refuses the category default of
a 3-D viewer beside a results list: there is one subject, and the strip beneath
it is not a gallery of choices but an answer, each tile carrying the cosine and
the cell it scored, opened to follow the answer into the picture.

OWN-WORLD: The gallery's own material — the near-black block on warm paper, the
12px mono caption, the tab row of native radios, the hairline rule, `--accent`
on the pressed state, the cue, the probe and the cosine. The photograph is the
only chromatic material; the accent washes it in `look-alikes` and never
replaces it. No new hue, face, radius, shadow or component language.

STORY: Every asset reveals itself — the still becomes a surface, slowly; turn it
and the readings separate; pick a part and the strip answers with the other five
photographs and the cells that matched. The cue is only the replay: it returns
when the reader goes back to the photograph, and the state control reaches every
state without it. With the
sandbox unreachable or WebGL missing, the page keeps its explanation, states
which part failed and draws nothing it was not sent.

FIRST VIEWPORT: The plate number and title (`VII. The perception`), the one
teaching sentence naming the two models and the five readings (the gist line in
read mode), and the ruled ledger above; then the stage on the dark ground,
sized to the sample's aspect and capped at 78% of the viewport height, with the
photograph and the one action over it and the filmstrip directly beneath.
Nothing above the stage but the masthead, the read-mode bar and that head.

FORM: The stage, an extension of the committed gallery world: shared `.block`,
`.fig`, `.cmds`, `.samples` and `.rig` atoms with plate-local `.stage__*` and
`.perception__*` rules for layout only. The stage block is the plate's `TRY IT`;
under it the inspector holds the plan at left and the `adjust` disclosure plus
the two honesty lines at right, collapsing to one column below 860px. The prose
below is the gallery's three teaching sections, with the deepest arithmetic of
`WHAT JUST HAPPENED` one disclosure away, then `THE EXACT COMMANDS`.

FINISH: unreviewed and undocumented is unfinished. This rework ends with the
detector state recorded below, the page read on the dev stack, the gallery's
entry left as its author wrote it, and every claim in the plate's copy still
true of the reply it describes.

---

## Detector

`impeccable detect website` reports the shared shell set only: `wide-tracking`
from the shell's tracked labels, five `cramped-padding` on `.block` sections,
and the `html`/`body` clipped-overflow pair. None of the plate's own former
findings survives — the `all-caps-body` note and the plate-local wide tracking
are gone — because the head's ledger is the shared `fig--as-written` caption,
which states its caps in the markup and carries no transform or label tracking,
and the plate's own labels sit at the committed 12px with 0.02–0.04em tracking.

Which of those are the world's own ground and which would need the shell
changed: the five `cramped-padding` findings are `.block`'s `clamp()` insets,
shared by every plate, and the `wide-tracking` note is the shell's tracked label
rule. Clearing either here would hide it on every plate, so they are the shell's
to change, not this page's. The clipped-overflow pair is the shell's positioned
skip link against `html`/`body`, raised by the same markup everywhere. Recorded,
not silenced.

## Finish review

Verdict: ships, against the shipped tree and the dev page
(`http://localhost:8080/demos/perception.html`). Against the direction contract
the plate is the stage it promised: the still is the sample's own bytes in the
sample's own aspect; the reveal starts by itself on every asset, on one 6.1-second
clock (900 ms dissolve, 3 000 ms depth, 2 200 ms camera) that ends by handing the
camera over, with the depth map's gradient arriving as light with the camera so
the lift is seen as the still's own displacement rather than a fade; the five readings are uniforms over one index; the
plan is the `role="img"` figure and the readout names the probe, the region and
the cosine; and the strip ranks by the pooled vector until a patch is pinned,
then by the grown region's mean against each picture's rows. The limits are on
the surface (`relative, monocular depth — no metres, one face · the box is the
region's extent, not a detector's output`) and repeated in the prose.

The decisions the page carries: the three states and the one reading control,
with `adjust` holding the rest; the reveal automatic on every asset and
cancellable by a pointer press, wheel, double-click or key, with the cue as its
replay; the state control as the reveal's owner, in one mix of three scalars; `look-alikes` as
a wash over a legible photograph rather than a replacement; fixed tolerances for
growth, so the region is a reading of the pin and not a sixth control; the
answer strip as the climax, one lazy lookup per other photograph and no ranking
call. The mesh control, the surface⇄cloud slider, point size, the depth slice,
spacing, the guides, the two region sliders, the reset button, `back to the
photograph` and the old multi-field HUD are deleted rather than hidden.

The checks that cannot be seen in a click also hold: the page reads with
scripting off, the plan and readout survive a browser without WebGL, the radios
are the labels' hit areas at 44px, and the camera's clamps (±25° yaw, ±15°
pitch, the fitted zoom range) are repeated in the prose because a single-view
reconstruction has one face. `just website-pages-check`,
`website-presets-check`, `website-published` and `website-version-check` pass.

**This change.** The surface is the one `openspec/changes/dinov3-parts-plate/`
builds: the stage-first plate, the depth model beside DINOv3, the indexed
surface with five readings, and the strip that answers with the other five
photographs. Its proposal and design are the record of the decisions above; this
brief is what the change's spec deltas and tasks are read against.
