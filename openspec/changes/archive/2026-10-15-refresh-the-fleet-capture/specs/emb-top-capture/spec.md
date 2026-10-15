# Spec Delta

## MODIFIED Requirements

### Requirement: The take stays in step with the dashboard's readouts

When the dashboard gains or changes a readout, every take of that dashboard SHALL be re-recorded in the same change, so each plate and each documented capture show it. The fleet view is one of those readouts: a change that alters the fleet sizing line, the attribution or the per-model panels re-records the fleet take as well, not only the single-node one. The recorded node SHALL run the default CPU policy unless the scenario documents an override.

#### Scenario: A new readout is published only from a new take

- **WHEN** the dashboard's readouts change and a committed take predates that change
- **THEN** no plate of that dashboard SHALL be published as showing the new readout until every take of it has been re-recorded

#### Scenario: A change that re-records one take re-records its sibling

- **WHEN** a change alters what the dashboard renders and re-records one of its takes
- **THEN** the other takes of the same dashboard SHALL be re-recorded in that change too, so the served plates do not name two builds

#### Scenario: The recorded node runs the default policy

- **WHEN** the take runs
- **THEN** the recorded node SHALL run with `cpu_budget: auto` and calibration enabled, so the plan the plate shows is the plan a default deployment selects

#### Scenario: The refreshed plate keeps its provenance

- **WHEN** a re-recorded plate is reviewed
- **THEN** every figure in it SHALL still come from that run's own sample log, and the plate's name SHALL change with its bytes

## ADDED Requirements

### Requirement: The published takes agree on the dashboard they recorded

Every recorded take the served pages carry SHALL declare the `emb-top` version it was recorded from beside its recording, and all of them SHALL declare the same one. A take left behind by a change that re-recorded its sibling SHALL fail the published-tree check rather than ship as a plate of an older dashboard. The check SHALL read the pages and the recorded labels only — no model, no server and no recorder — so it reaches its verdict wherever the tree is checked.

#### Scenario: The takes come from one dashboard

- **WHEN** the served pages are checked and every recorded take declares the same `emb-top` version
- **THEN** the check passes and reports the takes it compared and the version they agree on

#### Scenario: A take is left behind

- **WHEN** one plate declares an `emb-top` version the others do not
- **THEN** the check fails, naming every plate and the version it declares, so the take to re-record is the one the message points at

#### Scenario: The check needs no recorder

- **WHEN** the check runs without the recording tools, the models or a server
- **THEN** it still reaches its verdict, because it reads the served pages rather than re-recording anything
