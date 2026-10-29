# Spec Delta

## ADDED Requirements

### Requirement: A plate turns a photograph into a depth-shaded relief and searches it

The gallery SHALL carry a plate built on one image. It MUST send that image once to the dense-features model and once to a depth model, and MUST compute every particle it draws — position, colour, and similarity — from those two replies plus the image the browser itself sent. It MUST NOT draw a figure from stored data, and MUST refuse to draw rather than present a grid or a map whose shape disagrees with the server's declaration.

#### Scenario: Two products from two calls

- **WHEN** the plate loads a sample
- **THEN** it sends one image to the dense-features model for a pooled vector and a patch grid, and the same image to the depth model for a depth map
- **AND** the readout reports the grid's rows and dimension, the map's rows and dimension, and the server-measured time of each call

#### Scenario: One particle per photograph pixel

- **WHEN** the field is built
- **THEN** the particle count equals the pixel count of the bounded image the browser sent, and each particle's colour is sampled from that image, not from a palette or a ramp
- **AND** the photograph's aspect is preserved in the flat state, the reconstructed state and the mind state, so the figure is never a normalised square

#### Scenario: Depth is a separate model, and the plate says so

- **WHEN** the plate describes the reconstruction
- **THEN** it states that the depth is monocular, relative and approximate, that the reconstruction has one face, and that it is not DINOv3's own output

#### Scenario: The reconstruction returns to the photograph

- **WHEN** the depth exaggeration is zero
- **THEN** every particle sits in its image-plane position and the field is indistinguishable from the photograph

#### Scenario: One gesture moves between the two states

- **WHEN** the reader presses the reveal action, or picks another stop on the state control
- **THEN** the same particles interpolate between the image plane and the depth-derived relief without rebuilding geometry or issuing a command
- **AND** a change of state resets neither the probe nor the camera

#### Scenario: The depth map is used as a normal map

- **WHEN** the field is drawn as the relief
- **THEN** each particle's normal comes from the depth map's own gradient, and a fixed key light shades the surface, so surfaces turned toward the light are brighter
- **AND** the exaggeration control changes the shading with the geometry, because the slope is scaled by the same control

#### Scenario: The probe ranks the same part in the other photographs

- **WHEN** the reader hovers or pins a patch
- **THEN** the plate ranks that patch's cosine against every other patch in the picture and against every patch of the other committed photographs, and shows the best score and cell per picture
- **AND** opening a result loads that photograph and pins the patch the model chose, without a new model call beyond the grids it already fetched

#### Scenario: A ranking across pictures is only claimed for one model

- **WHEN** the plate compares patches from two photographs
- **THEN** both grids came from the same served model, and the copy says that a different checkpoint's vectors would not be comparable

#### Scenario: Similarity is the model's cosine, not a label

- **WHEN** the reader probes a patch
- **THEN** the highlight is the cosine between that patch's returned row and every other row, recomputed in the page without re-normalizing, and the readout names the probe cell, the best other cell and the cosine
- **AND** no class, category or object name appears anywhere in the figure

#### Scenario: The photograph is the accessible and the WebGL-free figure

- **WHEN** the browser cannot create a WebGL context, or the vendored module fails to load
- **THEN** the plate still draws a plan view of the same replies and the same readout, states that the field is unavailable, and fabricates nothing
- **AND** the plan is an image role whose label names the state, the probe's cell, its best match and the cosine

#### Scenario: Nothing animates unasked

- **WHEN** the reader prefers reduced motion
- **THEN** the field is drawn in its settled state in one frame, and no figure depends on an animation having run

#### Scenario: The samples keep their own scale and their own credit

- **WHEN** the picker shows the plate's photographs
- **THEN** each thumbnail shows the photograph's whole frame rather than a crop, and each committed file is public domain or CC0 with its work, author, licence and source recorded

#### Scenario: A refused or missing call draws nothing

- **WHEN** either model refuses the command or cannot be reached
- **THEN** the plate states the refusal or the outage, shows no field, and fabricates no value

#### Scenario: The page names no shape it did not read

- **WHEN** the plate's copy describes the grid, the map, the vector or the reply's size
- **THEN** the numbers come from the server's metadata and the replies, so a model the sandbox serves differently does not falsify the page

#### Scenario: The models' licences are named

- **WHEN** the plate describes the models behind the figure
- **THEN** it names the dense-features weights' licence and the depth model's licence and links them, and the site ships no model weights with the page

### Requirement: The plate leads with its figure

The plate SHALL present the stage before any prose block. Its head MUST carry the title, at most two sentences naming the two models, the TL;DR line and the ruled figure caption, and the stage, the filmstrip and the readout MUST follow immediately. No prose MUST be painted above the stage.

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

The plate SHALL keep the three-stop state control and the five-way reading control visible, and MUST place the depth exaggeration, the cut, the field density, the isolate toggle, the upload, and the probe's column and row inside a single disclosure of native controls. The camera MUST be clamped so no reachable angle exposes unsupported geometry. A tap or click on the figure MUST open the inspector and MUST keep the selection on the same patch across every state.

#### Scenario: Two live controls

- **WHEN** the plate is drawn without opening a disclosure
- **THEN** the state control and the reading control are the only live controls on the stage, and the replay cue is hidden while any state but the photograph shows

#### Scenario: Every tuning control is reachable

- **WHEN** a keyboard or screen-reader reader opens the disclosure
- **THEN** each tuning control is a native input with its own label, and changing one changes the figure

#### Scenario: The camera cannot open the backside

- **WHEN** the reader drags or keys past the clamp
- **THEN** the orbit stops within its yaw and pitch limits and the reconstruction still reads as one face

#### Scenario: The probe tracks the same particles

- **WHEN** the reader selects a patch, then reveals, orbits, or opens another photograph from the results
- **THEN** the same particles stay marked in the picture, and the readout names the cell, the best other cell and the cosine

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

### Requirement: The plate grows a region at fixed tolerances, isolates it, and reads one field five ways

The plate SHALL build a spatial region from the probed patch and SHALL expose the depth model's ordering and its gradient as views of the same field. Growth MUST be constrained by the served model's cosine, the photograph's colour and the depth map together, at fixed tolerances no control moves, and the region's mean vector MUST be the query the plate ranks with.

#### Scenario: The region grows from the model, not from a mask

- **WHEN** the reader probes a patch
- **THEN** the plate admits a neighbouring patch only where the model's cosine, the patch's colour and the depth map all agree, and the readout reports how many patches the region holds
- **AND** the query it ranks with is the mean of the region's unit vectors, normalized, so a boundary patch does not decide the answer

#### Scenario: Isolation keeps the region and the context

- **WHEN** the reader isolates the region
- **THEN** the region's particles keep full brightness, the rest of the field drops to a trace, and an approximate box encloses the region's extent in the same world coordinates the shader uses
- **AND** isolating never adds a particle, a colour or a surface that the photograph and the two replies do not already contain

#### Scenario: The normals view is the depth gradient, not a material

- **WHEN** the reader turns the normals view on
- **THEN** each particle is painted by its normal from the depth map's gradient — x red, y green, z blue — instead of the photograph's colour, and the relief is raised first if the photograph was still showing
- **AND** turning it off restores the photograph's own colours exactly

#### Scenario: The density tiers re-sample, never re-infer

- **WHEN** the reader changes the field density
- **THEN** the plate draws the same photograph at one of three deterministic particle counts and issues no command
- **AND** the patch grid, the depth map and the probe survive the change

#### Scenario: An uploaded photograph takes the same path

- **WHEN** the reader uploads an image
- **THEN** the browser bounds it, the same two models answer the same two calls, and the field keeps the uploaded photograph's aspect and colours
- **AND** nothing is stored on the server, and the plate ranks the uploaded part against the committed samples without a further call per picture

#### Scenario: A grown region is not a mask

- **WHEN** the plate says what the region is
- **THEN** it names the three tolerances that grew it, states that a segmentation would be a different model, and claims no category, object or hidden surface

### Requirement: The strip is the ranking, and it is arithmetic over replies the page already holds

The answer strip under the stage SHALL rank the six photographs against the picture on the stage. With a patch pinned it MUST rank every other photograph by that patch's cosine over that picture's own returned rows and MUST name the best cell in each; with nothing pinned it MUST rank them by the pooled vector's cosine. The lookups MUST be lazy and MUST be kept once fetched, and a ranking MUST NOT issue a call of its own.

#### Scenario: A pinned patch names its best cell in every photograph

- **WHEN** the reader pins a patch
- **THEN** each other photograph is scored by the highest cosine between the region's query and its own grid rows, and its tile names that cell with the cosine
- **AND** the picture on the stage is ranked first, naming its best other cell in the same grid

#### Scenario: With nothing pinned the whole pictures are ranked

- **WHEN** no patch is pinned
- **THEN** the tiles are ranked by the pooled vector's cosine, each other picture names that score rather than a cell, and the picture on the stage keeps its mark

#### Scenario: The strip asks once per picture, and never to rank

- **WHEN** a picture is first ranked
- **THEN** its grid is fetched once through the same patch call the stage uses and kept in memory, and no later ranking issues a request
- **AND** a picture that cannot be fetched is left out rather than guessed

#### Scenario: Look-alikes is a tint over a legible photograph

- **WHEN** the reader picks the look-alikes state
- **THEN** the photograph stays the ground, dimmed only as far as the accent needs, and the accent ramps steeply with the probe's own cosine while the grown region is washed into the same tint
- **AND** below the probe's own similarity nothing is painted, and turning the state off restores the photograph's colours

### Requirement: The plate reconstructs one indexed surface and reads it five ways

The plate SHALL build an indexed triangular mesh from the depth prediction over a regular image grid, and SHALL render the surface, a wireframe, camera-space normals, depth contours and a particle cloud from one shared vertex buffer. A cell MUST NOT be triangulated when any of its edges crosses a depth step beyond the tolerance, and the wireframe MUST NOT draw an edge the mesh does not have.

#### Scenario: The mesh is the depth, not a picture of it

- **WHEN** the plate draws the reconstruction
- **THEN** every vertex sits on the image plane displaced by the depth model's own value at its own UV, the aspect and colour are the photograph's, and the triangle count is two per accepted grid cell
- **AND** rejected cells leave an open edge with no filled interior

#### Scenario: One buffer, five readings

- **WHEN** the reader switches between surface, wireframe, normals, contours and cloud, or moves the cut
- **THEN** the geometry buffers are not rebuilt, only uniforms and indices change, and the selection and the camera survive the switch

#### Scenario: The wireframe is readable and true

- **WHEN** the wireframe is shown
- **THEN** it draws grid lines of the same mesh (a stride wide enough to read, with each stride cell's diagonal) and never an edge across a rejected cell
- **AND** it can be shown over the photograph, over a dimmed surface, or alone on the dark background

#### Scenario: The contours follow view depth

- **WHEN** the contours are on
- **THEN** the isolines are computed from the reconstructed depth from the camera outward, and they animate a slow sweep

#### Scenario: The cloud is the same vertices

- **WHEN** the reader switches to the cloud reading
- **THEN** the surface hides and the same vertices are drawn as points at the same reconstructed positions, with no rebuild and no random displacement

#### Scenario: The camera is bounded

- **WHEN** the reader orbits or zooms
- **THEN** yaw stays within about 25°, pitch within about 15°, and the motion is damped
