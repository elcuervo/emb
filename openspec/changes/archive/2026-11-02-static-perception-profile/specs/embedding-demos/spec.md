# Spec Delta

## MODIFIED Requirements

### Requirement: The profile is a measurement of the same surface

The plate SHALL draw the depth along one fixed default line through the photograph, in the same display units the vertex shader displaces by, and SHALL print that slice in the profile figure below the stage. The line MUST NOT be drawn on the photograph or the relief, and the reader MUST NOT move it.

#### Scenario: The graph cannot disagree with the geometry

- **WHEN** the profile is drawn
- **THEN** it samples the map with the same bilinear reader and rank equalisation the shader uses, and 100 sampled points match the shader's height function within 1e-6

#### Scenario: The readout is the measurement

- **WHEN** the profile is drawn
- **THEN** the readout prints the near and far values at the endpoints, the mean absolute slope along the line, and the span in frame-heights
- **AND** the copy says the axis is relative depth, because the model has no metres

#### Scenario: Dragging issues nothing

- **WHEN** a reader drags across the still
- **THEN** the drag places no profile and issues no request, because the still no longer carries a profile interaction

#### Scenario: The slice is fixed

- **WHEN** the plate draws a sample
- **THEN** the profile uses row 50 and the horizontal span from 8 to 92 percent, and a new sample uses the same slice
- **AND** no pointer or input changes the slice, and no request is issued for the profile

#### Scenario: The images carry no marker

- **WHEN** the still or the relief is drawn
- **THEN** neither carries the profile line or its endpoint dots, and the orange chart in the profile figure is the only drawing of the slice

### Requirement: The controls stay quiet

The plate SHALL keep the two-stop state control (`photograph`, `relief`) and the two-way reading control (`surface`, `normals`) visible, and MUST put the exaggeration, the cut, the density and the upload inside one disclosure. The camera MUST be clamped so no reachable angle exposes unsupported geometry.

#### Scenario: Two live controls

- **WHEN** the plate is drawn without opening a disclosure
- **THEN** the state control and the reading control are the only live controls on the stage, and no other stop — a look-alikes state, a parts reading, a probe or a crop — is offered

#### Scenario: Every tuning control is reachable

- **WHEN** a keyboard or screen-reader reader opens the disclosure
- **THEN** each tuning control is a native input with its own label, and changing one changes the figure

#### Scenario: The profile carries no control

- **WHEN** the disclosure is open or closed
- **THEN** no profile range input, drag target, or line marker is offered, and the profile chart is fixed at its default slice

#### Scenario: The camera cannot open the backside

- **WHEN** the reader drags or keys past the clamp
- **THEN** the orbit stops within its yaw and pitch limits and the reconstruction still reads as one face
