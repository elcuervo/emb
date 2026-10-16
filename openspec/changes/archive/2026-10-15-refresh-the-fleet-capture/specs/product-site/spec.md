# Spec Delta

## MODIFIED Requirements

### Requirement: The documentation surface carries the fleet run the way it carries the single-node run

The documentation surface SHALL present the recorded fleet run under the same
rules the single-node plate already obeys: the recording replayed as text in the
dashboard's own colours, held at a frame until the reader asks for it, with the
still frame painting before any script and a caption written by the capture. It
MUST NOT autoplay, MUST NOT serve an image or video of a dashboard, and MUST NOT
be an element carrying the site's current version. The fleet run SHALL be
recorded from the same dashboard the single-node plate was recorded from, so the
two plates show one build's output rather than two, and each plate SHALL declare
the `emb-top` version it was recorded from.

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
- **THEN** each names its own recorded run and its own figures, neither presents the other's numbers, and the `emb-top` version both declare is the same one

#### Scenario: A dashboard change reaches both plates

- **WHEN** a change alters what the dashboard renders and the fleet plate still shows the dashboard as it was before that change
- **THEN** the surface SHALL NOT present the two plates as the same dashboard until the fleet run has been re-recorded
