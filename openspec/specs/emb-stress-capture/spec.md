# emb-stress-capture Specification

## Purpose

A reproducible, scripted capture of a real `emb-stress` run against a fleet, in
the forms the repository needs it: the measured plate the documentation surface
shows and the capture the reference documentation carries — so the fleet tooling
is presented as it was measured rather than as a drawing from memory.

## Requirements

### Requirement: One command produces the fleet run, its plate and its capture

`just website-stressviz` SHALL be the only step required to produce the
documentation surface's `emb-stress` plate and the documentation's capture from
one real run: start the scenario's fleet, wait for every node to be ready, drive
it with `emb-stress`, read the fleet's own sizing lines from `emb-top`, publish
every form, and verify the served tree — with no manual step between start and
finish.

#### Scenario: A run from a clean checkout

- **WHEN** the command is run on a machine in the dev shell with the scenario's model files present
- **THEN** it exits 0, the plate carries the run's own figures and a caption, and the documented capture is newer than the previous one

#### Scenario: Missing models

- **WHEN** a model the scenario config names is not on disk
- **THEN** the command fails before starting the fleet, naming the missing file and the command that fetches it, and leaves the existing plate and capture untouched

#### Scenario: The fleet never settles

- **WHEN** a node does not answer its readiness probe within the command's deadline
- **THEN** the command fails with that node's log tail rather than recording a fleet that has not settled

### Requirement: The plate's numbers come from the run, not from the page

Every value the plate draws and every value its caption states SHALL be read from
the run's own result and sample log by the publishing step. The markup's values
MUST be written by the capture, so no rate, percentile, share, level or date is
typed into the page by hand.

#### Scenario: Regenerating the plate

- **WHEN** the command runs again
- **THEN** the caption, the axis labels and every drawn value are rewritten by the tool, and a hand-edited value in the page is overwritten

#### Scenario: A drawn value corresponds to the log

- **WHEN** the plate is inspected against the run's logs
- **THEN** each rate, percentile and share it draws is a value that run's log records

### Requirement: The plate shows what the pairing shows, and nothing the run did not measure

The plate SHALL present the run's offered-versus-observed behaviour — the share
each node was offered and the share it served — and the run's level curve with
its latency. It MUST NOT state a capacity, a service level, or a sizing
recommendation the run did not produce, and MUST NOT present a node's share of
the fleet as a server-side counter when it is the client's own count.

#### Scenario: The offered and the observed are distinguished

- **WHEN** the plate draws a node's share
- **THEN** the offered share is what `emb-stress` divided among the endpoints and the observed share is what each endpoint completed, and a node serving less than it was offered is drawn as such

#### Scenario: No invented verdict

- **WHEN** the run produced no sizing line for a node
- **THEN** the plate and its caption claim no sizing for that node and state only what was measured

#### Scenario: The levels are the requested ones

- **WHEN** the plate presents the level curve
- **THEN** the levels it shows are the levels the recorded invocation was given

### Requirement: The drawing says it is a drawing, and the run's own output is kept

The plate SHALL be composed from the site's existing figure atoms — ruled
graticule, mono caps labels, one accent signal — and SHALL be labelled as a
drawing of the run's measured values. The run's own result output and the
`emb-top` frames read during it SHALL be kept for that run.

#### Scenario: No new visual language

- **WHEN** the plate is rendered
- **THEN** it introduces no second hue, gradient, radius, shadow or font, and uses at most one accent signal

#### Scenario: The figures survive without the drawing

- **WHEN** the plate is read with its drawing removed
- **THEN** its caption states every value the plate draws, so no figure depends on the picture

#### Scenario: The run's evidence is on disk

- **WHEN** the run ends
- **THEN** its result output and the sizing lines read during it are retained for that run

### Requirement: A re-run replaces the previous artifact, and the working set stays unpublished

The documentation's capture SHALL be named from its own bytes, and the reference
that points at it SHALL be written by the capture, so a re-run replaces the
previous one and the repository holds exactly one. The rig, its scenario config,
its logs and its raw output MUST NOT be reachable at any served path.

#### Scenario: Two runs do not collide

- **WHEN** the command is run twice
- **THEN** it writes a different capture name the second time, the documentation references the second one, and the first no longer exists

#### Scenario: Working files are not published

- **WHEN** the published tree is checked
- **THEN** the rig's scripts, config and logs are not reachable at any served path, while the plate's markup and the documentation's capture are

### Requirement: The recorded fleet is a real fleet under a settled, deterministic load

The fleet SHALL be started from the rig's own scenario config, from local model
files, with the cache enabled, and SHALL answer ready before the run begins. The
load SHALL come from a committed, seeded `emb-stress` invocation whose levels and
dwell are stated with the plate, not from ambient or interactive traffic.

#### Scenario: No download during the run

- **WHEN** the fleet is started and driven
- **THEN** every node resolves its models from a local path and performs no model download

#### Scenario: The run is reproducible in shape

- **WHEN** the run is repeated with the same seed and levels
- **THEN** it offers the same request set, and the plate's shape is the shape a settled run produces

#### Scenario: Every node did work

- **WHEN** the run ends
- **THEN** each endpoint the scenario starts appears with non-zero completed work, and an endpoint that completed nothing is named rather than drawn as an idle row
