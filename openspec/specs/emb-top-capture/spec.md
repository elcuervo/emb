# emb-top-capture Specification

## Purpose
A reproducible, scripted capture of the live `emb-top` dashboard under load, in
the two forms the repository needs it: the run's own recording, replayed as text
on the landing page so a reader sees the dashboard move at the plate's own type,
and an animation for the documentation, where the same run can be watched in
colour — so the site can show the monitoring story as it actually renders instead
of as a hand-drawn approximation, and can be re-captured when the dashboard
changes.

## Requirements

### Requirement: One command produces the page's take and the documentation's capture

`just website-topviz` SHALL be the only step required to produce the landing
page's `emb-top` plate and the documentation's capture from a single real run. It
SHALL build the binaries it needs, start the node it records, take the recording,
derive the page's still frame from the same take, render the documentation's
capture from it, publish all of it, and report what it wrote, with no manual step
between start and finish.

#### Scenario: A run from a clean checkout

- **WHEN** the command is run on a machine in the dev shell with the model files present
- **THEN** it exits 0, the page carries a captured frame and a recorded take to play, and the documented capture exists and is newer than the previous one

#### Scenario: Missing models

- **WHEN** a model the scenario config names is not on disk
- **THEN** the command fails before recording, naming the missing file and the command that fetches it, and leaves the existing plate and capture untouched

#### Scenario: The node never becomes ready

- **WHEN** the node does not answer its readiness probe within the command's deadline
- **THEN** the command fails with the node's log tail rather than recording an empty dashboard

### Requirement: The page's plate plays the run's own recording, as text

The landing page's plate SHALL play the recorded run — the trimmed terminal
recording itself, replayed into the page — rather than a raster or vector capture
of it, so that its characters stay crisp at every viewport and every zoom and the
page carries no image of the dashboard. The recording it plays MUST be the take
of the published run. The site MUST serve no image of the dashboard.

#### Scenario: The plate is the recording

- **WHEN** the plate is inspected while playing
- **THEN** what it shows is the recorded take's own output — the same cast the documentation's capture was rendered from — drawn as text, with no image element standing in for the dashboard

#### Scenario: The plate is not a drawing

- **WHEN** the plate's content is compared against the recording it came from
- **THEN** every row, number and bar it shows is that recording's, and nothing is hand-authored

#### Scenario: No image of the dashboard ships

- **WHEN** the published tree is checked
- **THEN** it contains the recording and the capture's page markup, and no raster or video of the dashboard

### Requirement: The plate holds the still frame when motion is not welcome

The plate SHALL present the run's captured still frame whenever motion is not
available or not wanted: with scripting unavailable, and for a reader whose
motion preference is `reduce`. It SHALL autoplay only where motion is welcome,
SHALL provide a control to stop and resume the playback, and SHALL NOT leave
motion that a reader cannot stop.

#### Scenario: Reduced motion

- **WHEN** the page is opened with `prefers-reduced-motion: reduce`
- **THEN** the plate does not autoplay and holds a still frame of the same run

#### Scenario: No scripting

- **WHEN** the page is rendered with scripting unavailable
- **THEN** the plate still shows the captured frame at the plate's own type size, and the caption and the run's figures are complete

#### Scenario: Motion can be stopped

- **WHEN** the plate is playing
- **THEN** a control pauses it, and the plate holds its frame until resumed

### Requirement: The plate is representative

The still frame the plate holds SHALL be a moment of the run where the models the
scenario exercises are all active and the run's history has filled the
dashboard's window, rather than the first or last frame.

#### Scenario: The still is taken mid-run

- **WHEN** the frame the plate holds is compared against the recording
- **THEN** it is one moment of the recorded run's sustained phase, not its idle opening or its drain

### Requirement: The player that replays the recording is vendored and pinned

The page SHALL serve the replaying player from its own origin, without a
third-party request at runtime. The vendored player's bytes MUST be pinned by
hash to a specific release and MUST carry that release's version in their
filenames, and the site's publish check MUST account for them.

#### Scenario: No runtime third party

- **WHEN** the page is loaded and the plate plays
- **THEN** every request the plate makes is to the site's own origin

#### Scenario: The vendor bytes are pinned

- **WHEN** the vendored player files are re-derived from the pinned release
- **THEN** they are byte-identical to what the site serves

#### Scenario: The vendor files are expected

- **WHEN** the published tree is checked
- **THEN** the player's script and stylesheet are in the served set and no served file is unexpected

### Requirement: The documented capture is edited, themed and legible

The capture committed for the documentation SHALL be rendered at a geometry whose
text stays legible at the width the documentation presents it, in the site's own
palette rather than a default terminal theme, and SHALL be shorter than the raw
recording.

#### Scenario: The palette is the site's

- **WHEN** the documented capture is rendered
- **THEN** its ground and body text match the site's dark plate and paper tokens, and the request-rate stream is drawn in the accent colour

#### Scenario: The capture is shorter than the take

- **WHEN** the recording and the documented capture durations are compared
- **THEN** the capture is the shorter of the two and holds its final frame

#### Scenario: The capture fits the documentation's measure

- **WHEN** the capture is rendered at the width `docs/operations.md` is read at
- **THEN** its text is legible without zooming

### Requirement: The take runs against one warm node serving several models

The recorded node SHALL serve more than one model from local files, with the
cache enabled, and SHALL be fully loaded and reachable before recording starts,
so that the plate shows per-model activity rather than a single row or a loading
screen.

#### Scenario: Several models are exercised

- **WHEN** the traffic scenario runs
- **THEN** at least three models appear in the plate with non-zero request activity

#### Scenario: No download during the take

- **WHEN** the recording is running
- **THEN** the node resolves every model from a local path and performs no model download

#### Scenario: Models are readable in the plate

- **WHEN** a model is named in the scenario config
- **THEN** its name is short enough that the dashboard does not truncate it in the recording

### Requirement: The traffic scenario is scripted and produces a readable narrative

The load SHALL come from a committed, deterministic scenario rather than ambient
or interactive traffic. It SHALL vary text length so latency percentiles move,
repeat texts so the cache ratio moves, and stagger per-model start and stop so
the activity heatmap shows bands rather than a uniform block.

#### Scenario: The take has a beginning, a middle and an end

- **WHEN** the plate plays
- **THEN** the dashboard is seen idle, then under rising load, then draining, within the take's duration

#### Scenario: Cache and latency both move

- **WHEN** the scenario completes
- **THEN** the machine-readable sample log shows a cache hit ratio that rises and latency percentiles that respond to text length

#### Scenario: No errors are injected

- **WHEN** the scenario is authored or run
- **THEN** it sends no request intended to produce an error, and no surface presents an error rate the node did not report

### Requirement: The take presents only measured values

Every figure visible in the plate, the still frame and the documented capture
SHALL come from the recorded run. The path from the recorded terminal output to
the page and to the documented capture MUST NOT alter, substitute or complete
numeric content.

#### Scenario: No fabricated metrics

- **WHEN** the plate and the documented capture are inspected against the run's provenance logs
- **THEN** the model names, rates and counters they show correspond to that run

#### Scenario: The error story is omitted rather than invented

- **WHEN** the recorded run produces no per-model error
- **THEN** no surface shows an error rate and no error highlight

### Requirement: A published artifact's name changes when its bytes change

The take the page plays and the documented capture SHALL each be named from their
own content, and the references to them SHALL be written by the capture. A
re-record SHALL replace those references and remove the previous files, so the
repository holds exactly one of each.

#### Scenario: Two takes do not collide

- **WHEN** the command is run twice
- **THEN** it writes a different cast name and a different capture name the second time, and both surfaces reference the second ones

#### Scenario: The previous take is gone

- **WHEN** the command finishes
- **THEN** the previous cast and the previous capture no longer exist and nothing references them

### Requirement: The take keeps its provenance

The rig SHALL retain the raw terminal recording it produced and a machine-readable
sample log of the same run, and the repository SHALL carry the run's date, the
captured `emb-top` version, the model count and the node beside the plate.

#### Scenario: Provenance survives the take

- **WHEN** the command finishes
- **THEN** the raw recording and the sample log are on disk for that run, and the plate is captioned with the run's date, version, model count and address

#### Scenario: Working files are not published

- **WHEN** the published tree is checked
- **THEN** the raw recording and the rig's scripts are not reachable at any served path, while the trimmed take the page plays is

### Requirement: Playing the take is not required to build or serve the site

The recording toolchain MUST be confined to the website development shell. A
checkout that only serves, deploys or verifies the site SHALL NOT need a recorder
or a player toolchain installed, because the player's bytes and the take are both
in the tree.

#### Scenario: The site serves without a recorder

- **WHEN** the site's own checks run without the recording tools available
- **THEN** they pass unchanged
