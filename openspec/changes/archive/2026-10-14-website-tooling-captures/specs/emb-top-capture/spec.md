# Spec Delta

## ADDED Requirements

### Requirement: The fleet take is published in the same forms as the single-node take

The rig SHALL publish the fleet recording as a recording, not only as an
animation for the repository's documentation: the trimmed take into the served
tree under a name made of its own bytes, its still frame, and the plate that
replays it on the documentation surface. The animated capture for the repository
documentation SHALL continue to be produced from that same take.

#### Scenario: One take, two renderings

- **WHEN** the fleet capture in `docs/assets/` and the take the page plays are compared
- **THEN** both come from the same recorded run and neither is maintained by hand

#### Scenario: A re-record replaces the fleet take

- **WHEN** the fleet target is run again
- **THEN** the served cast and the documentation's capture are both replaced by the new run's, the previous files are removed, and the repository holds exactly one of each

#### Scenario: The served set accounts for the new take

- **WHEN** the published tree is checked
- **THEN** the fleet take is in the served set, the rig's raw recordings are not served, and no served file is unexpected

### Requirement: The fleet take ships as the recording's own text

The fleet run SHALL be presented as the recording's own characters drawn as text,
selectable and sharp at any zoom. Because the documentation surface SHALL serve
no image of a dashboard, no raster, video or screenshot of the dashboard MAY
stand in for the fleet run.

#### Scenario: The fleet plate is a recording

- **WHEN** the fleet plate is inspected
- **THEN** it presents the recorded run's own characters as text, and no image or video element stands in for the dashboard

#### Scenario: No image of the fleet's dashboards ships

- **WHEN** the published tree is checked
- **THEN** it contains the fleet recording and the plate's markup, and no raster or video of a dashboard

### Requirement: The fleet take shows a fleet under load

The recorded fleet SHALL be two or more real nodes, started from local model
files with the cache enabled, watched while committed traffic drives them, so
that the plate shows a fleet's distribution rather than one node's rates.

#### Scenario: More than one node appears

- **WHEN** the traffic scenario runs
- **THEN** every node the scenario starts appears in the recording with non-zero request activity

#### Scenario: The distribution is visible

- **WHEN** the plate plays
- **THEN** the fleet's per-node traffic and its share of the total are readable, and the aggregate is the sum of what the nodes served
