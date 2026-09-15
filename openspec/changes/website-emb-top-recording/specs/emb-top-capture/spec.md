## Purpose

A reproducible, scripted capture of the live `emb-top` dashboard under load, so
the site can show the monitoring story as it actually renders instead of as a
hand-drawn approximation — and so the capture can be thrown away and re-taken
when the dashboard changes.

## ADDED Requirements

### Requirement: One command produces the plate

`just website-topviz` SHALL be the only step required to produce the site's
`emb-top` plate from a real run. It SHALL build the binaries it needs, start the
node it records, take the capture, render the shipped assets, and report the
resulting artifact names, with no manual step between start and finish.

#### Scenario: A run from a clean checkout

- **WHEN** the command is run on a machine in the dev shell with the model files present
- **THEN** it exits 0 and the serving tree contains a newly named animated asset and its poster frame

#### Scenario: Missing models

- **WHEN** a model the scenario config names is not on disk
- **THEN** the command fails before recording, naming the missing file and the command that fetches it, and leaves the existing plate assets untouched

#### Scenario: The node never becomes ready

- **WHEN** the node does not answer its readiness probe within the command's deadline
- **THEN** the command fails with the node's log tail rather than recording an empty dashboard

### Requirement: The capture runs against one warm node serving several models

The recorded node SHALL serve more than one model from local files, with the
cache enabled, and SHALL be fully loaded and reachable before recording starts,
so that the animation shows per-model activity rather than a single row or a
loading screen.

#### Scenario: Several models are exercised

- **WHEN** the traffic scenario runs
- **THEN** at least three models appear in the capture with non-zero request activity

#### Scenario: No download during the take

- **WHEN** the capture is running
- **THEN** the node resolves every model from a local path and performs no model download

#### Scenario: Models are readable in the panel

- **WHEN** a model is named in the scenario config
- **THEN** its name is short enough that the dashboard does not truncate it in the capture

### Requirement: The traffic scenario is scripted and produces a readable narrative

The load SHALL come from a committed, deterministic scenario rather than ambient
or interactive traffic. It SHALL vary text length so latency percentiles move,
repeat texts so the cache ratio moves, and stagger per-model start and stop so
the activity heatmap shows bands rather than a uniform block.

#### Scenario: The capture has a beginning, a middle and an end

- **WHEN** the shipped clip plays
- **THEN** the dashboard is seen idle, then under rising load, then draining, within the clip's duration

#### Scenario: Cache and latency both move

- **WHEN** the scenario completes
- **THEN** the machine-readable sample log shows a cache hit ratio that rises and latency percentiles that respond to text length

#### Scenario: No errors are injected

- **WHEN** the scenario is authored or run
- **THEN** it sends no request intended to produce an error, and the capture presents no error rate the node did not report

### Requirement: The recording presents only measured values

Every figure visible in the shipped animation SHALL come from the recorded run.
The pipe from the recorded terminal output to the shipped asset MUST NOT alter,
substitute or complete numeric content.

#### Scenario: No fabricated metrics

- **WHEN** the shipped animation is inspected against the capture's provenance logs
- **THEN** the model names, rates and counters it shows correspond to that run

#### Scenario: The error story is omitted rather than invented

- **WHEN** the recorded run produces no per-model error
- **THEN** the animation shows no error rate and no error highlight

### Requirement: The shipped animation is edited, themed and legible

The rig SHALL render the capture at a geometry where the dashboard's every column
fits without wrapping, at a type size that stays legible at the width the plate
occupies on the page. It SHALL apply the site's own palette rather than a default
terminal theme, and SHALL cut dead time so the shipped clip is shorter than the
recording.

#### Scenario: Nothing wraps or clips

- **WHEN** the shipped animation is rendered at the recording's geometry
- **THEN** no dashboard row wraps and no column is cut off

#### Scenario: The palette is the site's

- **WHEN** the animation is rendered
- **THEN** the plate background and body text match the site's dark plate and paper tokens, and the request-rate stream is drawn in the accent colour

#### Scenario: The clip is shorter than the take

- **WHEN** the recording and the shipped animation durations are compared
- **THEN** the shipped animation is the shorter of the two and holds its final frame

### Requirement: The shipped asset name changes when its bytes change

The animated asset and its poster SHALL be named from their own content, and the
page SHALL reference those names. A re-record SHALL replace the page's references
and remove the previous take's files, so the serving tree holds exactly one take.

#### Scenario: Two takes do not collide

- **WHEN** the command is run twice
- **THEN** it writes a different artifact name the second time and the page references the second one

#### Scenario: The served set stays exact

- **WHEN** the site's published-tree check runs after a re-record
- **THEN** it passes, with no orphaned take left in the serving tree

### Requirement: The capture keeps its provenance off the served surface

The rig SHALL retain the terminal recording it produced and a machine-readable
sample log of the same run, in the unserved tools tree, so a reviewer can check
what the animation shows.

#### Scenario: Provenance survives the take

- **WHEN** the command finishes
- **THEN** the cast and the sample log for that run are on disk in the unserved tools directory

#### Scenario: Provenance is not served

- **WHEN** the published tree is checked
- **THEN** the recordings and the rig's scripts are not reachable at any served path

### Requirement: Recording is not required to build or serve the site

The recording toolchain MUST be confined to the website development shell. A
checkout that only serves, deploys or verifies the site SHALL NOT need a recorder
installed.

#### Scenario: The site builds without a recorder

- **WHEN** the site's own checks run without the recording tools available
- **THEN** they pass unchanged
