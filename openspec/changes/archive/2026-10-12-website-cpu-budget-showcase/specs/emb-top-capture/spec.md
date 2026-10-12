## ADDED Requirements

### Requirement: The take stays in step with the dashboard's readouts

When the dashboard gains or changes a readout, the take SHALL be re-recorded in the same change, so the plate and the documented capture show it. The recorded node SHALL run the default CPU policy unless the scenario documents an override.

#### Scenario: A new readout is published only from a new take

- **WHEN** the dashboard's readouts change and the committed take predates that change
- **THEN** the plate SHALL NOT be published as showing the new readout until the take has been re-recorded

#### Scenario: The recorded node runs the default policy

- **WHEN** the take runs
- **THEN** the recorded node SHALL run with `cpu_budget: auto` and calibration enabled, so the plan the plate shows is the plan a default deployment selects

#### Scenario: The refreshed plate keeps its provenance

- **WHEN** a re-recorded plate is reviewed
- **THEN** every figure in it SHALL still come from that run's own sample log, and the plate's name SHALL change with its bytes
