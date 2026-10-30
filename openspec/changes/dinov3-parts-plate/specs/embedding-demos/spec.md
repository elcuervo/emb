# Spec Delta

## ADDED Requirements

### Requirement: A plate turns one photograph into a depth map, a relief and three readings of that map

The gallery SHALL carry a plate built on one photograph and **one** model: the photograph is sent once to a depth model, and every figure the plate draws MUST be computed from that reply plus the image the browser itself sent. The plate MUST NOT call a second model, MUST NOT draw a figure from stored data, and MUST refuse to draw rather than present a map whose shape disagrees with the server's declaration.

#### Scenario: One call, one map, every figure after it

- **WHEN** the plate loads a sample
- **THEN** it sends one bounded image to the depth model and receives a 266 × 266 relative map as packed float32
- **AND** the relief, the map figure, the profile and the focus are all computed from that reply and the browser's own pixels, and the readout reports the map's rows and dimension with the server-measured time

#### Scenario: A refused or missing call draws nothing

- **WHEN** the model refuses the command or cannot be reached
- **THEN** the plate states the refusal or the outage, shows no relief, no map, no profile and no focus, and fabricates no value

#### Scenario: The page names no shape it did not read

- **WHEN** the plate's copy describes the map, the relief or the reply's size
- **THEN** the numbers come from `EMB.INFO` and the replies, so a model served at another size does not falsify the page

#### Scenario: The photograph is the figure, and it keeps its own scale

- **WHEN** the field is built
- **THEN** the vertex count is `ceil(bounded width / step) × ceil(bounded height / step)` for the tier drawn (the step comes from the density control, and a narrow stage caps the tier at the middle one), each vertex's colour is sampled from that image, and the photograph's aspect is preserved in the relief, the map and the plan

#### Scenario: The reconstruction returns to the photograph

- **WHEN** the depth exaggeration is zero
- **THEN** every vertex sits in its image-plane position and the field is indistinguishable from the photograph

#### Scenario: The depth map is used as a normal map

- **WHEN** the field is drawn as the relief
- **THEN** each vertex's normal comes from the depth map's own gradient, a fixed key light shades the surface, and the exaggeration control scales the slope and the lift together

#### Scenario: The still is exactly the source

- **WHEN** the relief is not raised, or the state control returns to the photograph
- **THEN** the photograph is drawn from the bytes the browser sent and every shading term multiplies by one

#### Scenario: The models' licence is named

- **WHEN** the plate describes the model behind the figure
- **THEN** it names the depth model's licence and links it, and the site ships no model weights with the page
### Requirement: The map is a figure, at the picture's own size

The plate SHALL draw the depth reply as itself: a raster beside the photograph on the same aspect, one cell per map pixel, with a native `raw` / `rank-equalised` toggle and a readout that reports the map's shape, its range and the exact float under the pointer.

#### Scenario: The reply is drawn, not summarised

- **WHEN** the map figure is drawn
- **THEN** every cell is one value from the reply, the ramp's limits are the map's own p5/p95 and both are printed, and the pointer's readout equals `depth[row × dim + col]` exactly

#### Scenario: The transform is shown, not implied

- **WHEN** the reader toggles `raw` to `rank-equalised`
- **THEN** the raster redraws with the same rank map the relief uses, and the caption says why the geometry needs it

#### Scenario: The map is the accessible figure

- **WHEN** WebGL is unavailable, or a screen reader reads the figure
- **THEN** the map canvas remains the `role="img"` figure with a label carrying the shape and the range, and no value is invented
### Requirement: The profile is a measurement of the same surface

The plate SHALL let the reader define a line on the photograph — by dragging it, or with two native range inputs inside the disclosure — and SHALL draw the depth along that line in the same display units the vertex shader displaces by, with both endpoints marked on the photograph and on the relief.

#### Scenario: The graph cannot disagree with the geometry

- **WHEN** the profile is drawn
- **THEN** it samples the map with the same bilinear reader and rank equalisation the shader uses, and 100 sampled points match the shader's height function within 1e-6

#### Scenario: The readout is the measurement

- **WHEN** the line is in place
- **THEN** the readout prints the near and far values at the endpoints, the mean absolute slope along the line, and the span in frame-heights
- **AND** the copy says the axis is relative depth, because the model has no metres

#### Scenario: Dragging issues nothing

- **WHEN** the reader drags the line or moves an endpoint input
- **THEN** no request is issued, because the profile is arithmetic over the map the page already holds
### Requirement: The focus is the depth map as a depth of field, and its failures are left in

The plate SHALL offer a focus interaction built from the same map: a focus distance expressed as a percentile of the map and an aperture expressed as the width of the sharp band, blurring the photograph by its distance from that band, with a sharpness measurement printed.

#### Scenario: The plane is sharp and the rest is not

- **WHEN** the reader sets a focus distance and an aperture
- **THEN** the photograph is blurred in six quantised bands drawn with `ctx.filter` and masked by the map, and the readout prints the focus percentile, the band's width and the sharpness ratio between the focused band and everything outside it

#### Scenario: A weak result is not shipped

- **WHEN** the focus figure is measured on a shipped picture
- **THEN** the sharpness ratio is at least 3, a change renders in at most 40 ms, and the interaction issues no request

#### Scenario: The map's mistakes stay on screen

- **WHEN** the map is wrong — a window, a thin branch, an opening — the blur follows the mistake
- **THEN** the caption states that the figure is the map used as a lens and not a lens, and no value is corrected by hand
### Requirement: The plate reconstructs one indexed surface, reads it two ways, and lights it as a solid

The plate SHALL build an indexed triangular mesh from the depth prediction over a regular image grid, and SHALL render **the surface** and **camera-space normals** from one shared vertex buffer. A cell MUST NOT be triangulated when any of its edges crosses a depth step beyond the tolerance. The mesh SHALL be lit by the depth map's own gradient and SHALL carry cast and contact shadows from that same height field.

#### Scenario: The mesh is the depth, not a picture of it

- **WHEN** the plate draws the reconstruction
- **THEN** every vertex sits on the image plane displaced by the depth model's own value at its own UV, and the triangle count is two per accepted grid cell

#### Scenario: One buffer, two readings

- **WHEN** the reader switches between surface and normals, or moves the cut
- **THEN** the geometry buffers are not rebuilt, only uniforms and indices change, and the camera survives the switch

#### Scenario: The flow is the depth cue, and it is the only one

- **WHEN** the surface is drawn
- **THEN** a front of light travels through the view depth every eleven seconds, and no isoline, wireframe or particle reading is offered beside it

#### Scenario: The camera is bounded

- **WHEN** the reader orbits or zooms
- **THEN** yaw stays within about 25°, pitch within about 15°, and the motion is damped
### Requirement: The depth model also lights the field, through its gradient

The plate SHALL use the depth model's own ordering as light once the field leaves the photograph plane: nearer particles brighter and slightly warmer, farther particles dimmer and slightly cooler (each within about a tenth of a channel), and SHALL leave the still state unlit so the photograph is rendered exactly as sent.

#### Scenario: The still is not lit

- **WHEN** the state control is at the photograph end
- **THEN** the lighting term is zero and the frame is the photograph as the browser bounded it, with no added brightness or falloff

#### Scenario: The volume reads head-on

- **WHEN** the state control is at the world end and the camera has not been dragged
- **THEN** the near end of the depth map is visibly brighter and the far end visibly dimmer, so foreground, middle ground and background separate without an orbit

#### Scenario: The still is exactly the source

- **WHEN** the relief is not raised — before the reveal, or after the state control returns to the photograph, or a fresh sample arrives in that state
- **THEN** the photograph is drawn from the bytes the browser sent, and every shading term multiplies by one
### Requirement: The plate leads with its figure

The plate SHALL be named **The perception** wherever the site names it — the page's title and metadata, its head, the gallery's index entry and the gallery's own list of plates — and SHALL present the stage before any prose block. Its head MUST carry the title, at most two sentences naming the one model the plate runs, the TL;DR line and the ruled figure caption, and the stage, the filmstrip and the readout MUST follow immediately. No prose MUST be painted above the stage.

#### Scenario: The first screen is the photograph

- **WHEN** the plate loads on a desktop viewport
- **THEN** the visible surface above the fold is the title, at most two sentences, the stage with its photograph already handing over to the relief, the filmstrip and the readout
- **AND** no prose paragraph is painted above the stage
### Requirement: The copy below the stage is the gallery's own

The plate SHALL use the gallery's five sections in its own order — the stage labelled as the try-it block, then what you are looking at, what just happened, why it matters, and the exact commands — and only the deepest arithmetic MAY sit in one native disclosure inside what just happened.

#### Scenario: The copy is the gallery's copy

- **WHEN** the reader scrolls past the stage
- **THEN** the stage carries the try-it label and the plate answers the four headings below it — what you are looking at, what just happened, why it matters, the exact commands — in that order, with its own two models in place of theirs
- **AND** read mode keeps the try-it stage and the exact commands and hides the three prose sections

#### Scenario: The long form survives without scripting

- **WHEN** scripting is unavailable, or a reader opens the disclosure
- **THEN** the page still states the two calls, both reply shapes, the cache, the one-face limit, the licences and the exact commands
### Requirement: The plate reveals every asset by itself

The plate SHALL run the reveal with no action from the reader once an asset's replies are drawn, whenever the plate is showing that asset's photograph. It MUST run on one clock — a dissolve, a depth awakening and a bounded camera orbit that ends by handing the camera over — and MUST bring the depth gradient's light, with the distance falloff, in with the camera phase, so the lift is seen as the still's own displacement rather than a fade through black.

#### Scenario: The clock is 900 · 3 000 · 2 200

- **WHEN** the sequence runs
- **THEN** the dissolve takes 900 ms, the depth awakening 3 000 ms and the camera 2 200 ms, and the gradient's light is zero until the camera phase begins

#### Scenario: The plate reveals the asset it just loaded

- **WHEN** a sample's or an upload's replies are drawn while the plate is showing a photograph
- **THEN** the sequence starts with no reader action, and the dissolve hands the photograph over to its own vertices

#### Scenario: A switch reveals the asset it opened

- **WHEN** the reader picks another sample
- **THEN** the state resets to that sample's photograph and the sequence runs again from there

#### Scenario: Reduced motion is respected

- **WHEN** the reader prefers reduced motion
- **THEN** the plate draws the settled relief in one frame, no orbit runs, and no figure depends on an animation having played
### Requirement: The reveal can be taken over, and the cue is the replay

The plate MUST cancel the sequence on any pointer press, wheel, double-click or key input, and MUST let the state and reading controls take it over from the frame it had reached. The `Reveal perception` action MUST be a replay: hidden while any state but the photograph shows, and back when the reader returns to the photograph.

#### Scenario: The reveal transform can be taken over

- **WHEN** the reader moves the state control or drags the stage while a reveal is running
- **THEN** the field continues from the frame it had reached, and no second animation starts

#### Scenario: The cue is the replay, and nothing needs it

- **WHEN** the sequence has run and the reader returns to the photograph
- **THEN** the `Reveal perception` action reappears and pressing it runs the sequence again
- **AND** every asset has already been revealed without it, so the photograph is complete, the state control still reaches every state, and the inspection still works whether or not the reader ever presses it
### Requirement: The controls stay quiet

The plate SHALL keep the two-stop state control (`photograph`, `relief`) and the two-way reading control (`surface`, `normals`) visible, and MUST put the exaggeration, the cut, the density, the upload and the profile's two endpoints inside one disclosure. The camera MUST be clamped so no reachable angle exposes unsupported geometry.

#### Scenario: Two live controls

- **WHEN** the plate is drawn without opening a disclosure
- **THEN** the state control and the reading control are the only live controls on the stage, and no other stop — a look-alikes state, a parts reading, a probe or a crop — is offered

#### Scenario: Every tuning control is reachable

- **WHEN** a keyboard or screen-reader reader opens the disclosure
- **THEN** each tuning control is a native input with its own label, and changing one changes the figure

#### Scenario: The camera cannot open the backside

- **WHEN** the reader drags or keys past the clamp
- **THEN** the orbit stops within its yaw and pitch limits and the reconstruction still reads as one face
### Requirement: The library is Unsplash, chosen for the depth it carries

The plate's six photographs MUST be Unsplash photographs arriving through their Wikimedia Commons CC0 mirrors, credited in `CREDITS.md` with work, photographer, Unsplash page and mirror, and each MUST clear the depth gate before it ships. Depth is the only selection axis: the map must span the range with both deciles occupied, so the relief has a foreground, a middle ground and a background.

#### Scenario: A candidate earns its place on the map

- **WHEN** a photograph is considered for the plate
- **THEN** its own 266 × 266 map is reduced to the p5/p95 spread, the near and far decile occupancy and the gradient energy, and it is kept only where the spread is at least 0.60 with both deciles at least 0.01

#### Scenario: The licence path is the mirror's, not a stock site's

- **WHEN** a photograph ships
- **THEN** the committed bytes are the Commons mirror, its licence is read from Commons' own metadata as CC0 or public domain, and the Unsplash page is credited as the source
- **AND** the tree ships no hotlink, no stock-site licence text and no model weights
