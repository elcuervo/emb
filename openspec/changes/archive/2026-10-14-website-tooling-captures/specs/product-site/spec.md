# Spec Delta

## ADDED Requirements

### Requirement: The documentation surface carries the fleet run the way it carries the single-node run

The documentation surface SHALL present the recorded fleet run under the same
rules the single-node plate already obeys: the recording replayed as text in the
dashboard's own colours, held at a frame until the reader asks for it, with the
still frame painting before any script and a caption written by the capture. It
MUST NOT autoplay, MUST NOT serve an image or video of a dashboard, and MUST NOT
be an element carrying the site's current version.

#### Scenario: The fleet plate is the recording, in colour

- **WHEN** the documentation surface is rendered with scripting available
- **THEN** the fleet plate shows the recorded run drawn by the player with the dashboard's own colours, and not an uncoloured text frame and not a picture

#### Scenario: Motion is requested, not imposed

- **WHEN** the documentation surface is opened with scripting available and the reader has not started the fleet recording
- **THEN** the plate holds a frame of the run, nothing is playing, and the player's own control starts it

#### Scenario: No scripting, no empty plate

- **WHEN** the documentation surface is rendered with scripting unavailable or failing
- **THEN** both tooling plates show their runs' still frames, captions and figures, and the page's content is complete

#### Scenario: No image of a dashboard is served

- **WHEN** the published tree is checked
- **THEN** it contains the two recorded takes and no raster or video of a dashboard

#### Scenario: The two plates cannot disagree about their runs

- **WHEN** the single-node plate and the fleet plate are compared
- **THEN** each names its own recorded run and its own figures, and neither presents the other's numbers

### Requirement: The measured load figure is a drawing, and it says so

The documentation surface SHALL present the `emb-stress` measurement as a figure
drawn from one recorded run, composed from the site's existing figure atoms with
at most one accent signal, captioned with every value it draws and with the
command that reproduced it. It MUST be labelled as a drawing of measured values,
MUST NOT imply a measurement the page took, and MUST NOT be an image.

#### Scenario: The figure is captioned with its values

- **WHEN** the figure is reviewed
- **THEN** its caption states the levels, shares and percentiles it draws rather than a qualitative summary, and names the run's fleet and models

#### Scenario: The figure adds no new visual language

- **WHEN** the figure is rendered
- **THEN** it uses the poster's existing atoms — ruled graticule, mono caps labels, small filled marks, one accent — and introduces no second hue, gradient, radius or shadow

#### Scenario: The figure does not claim a verdict

- **WHEN** the figure presents the fleet under load
- **THEN** it states what the run measured and names no capacity, service level or purchase it did not observe

#### Scenario: The figure is not the only number

- **WHEN** the site makes a performance or capacity claim
- **THEN** at least one figure remains traceable to a `BENCHMARK.md` run with its reproduction command, independently of any captured plate

#### Scenario: The figure survives without its drawing

- **WHEN** the figure is read with its drawing removed
- **THEN** its caption and its text state every value the drawing presents, and the surface's claims remain complete
