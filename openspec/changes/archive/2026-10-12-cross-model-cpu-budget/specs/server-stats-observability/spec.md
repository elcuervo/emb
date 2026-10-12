## ADDED Requirements

### Requirement: Process CPU plan is observable

EMB.STATS SHALL report the process CPU allowance and source, budget, active admission weight, and peak admission weight. Reading EMB.STATS SHALL NOT initialize lazy models.

#### Scenario: Process plan

- **WHEN** EMB.STATS is requested
- **THEN** the response SHALL include allowance, budget, active, and peak values

### Requirement: Selected model layout is observable

EMB.INFO SHALL report the selected intra-op thread width and actual materialized embedding session count. Reading EMB.INFO SHALL NOT initialize a lazy embedding pool.

#### Scenario: Loaded calibrated model

- **WHEN** EMB.INFO is requested for a loaded model
- **THEN** the reply SHALL include its actual worker count and selected thread width
