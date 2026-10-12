## ADDED Requirements

### Requirement: The landing presents the process CPU budget

The landing SHALL present the process CPU budget and the startup calibration as one reading: the detected allowance and its source, the admitted budget, and the per-model session count and width the calibration selected. A throughput figure SHALL name the policy that produced it and SHALL trace to a recorded run or a named recipe.

#### Scenario: The plan is named

- **WHEN** a reader reaches the capability block that presents the budget
- **THEN** it states the detected allowance and its source, the admitted budget, the selected sessions and width per model, and what the server does when no candidate validates

#### Scenario: A figure has provenance

- **WHEN** the landing states a throughput, latency or CPU figure
- **THEN** it names the policy that produced it (`cpu_budget: auto` or `cpu_budget: off`), the machine and the corpus, and links the recipe that reproduces it

#### Scenario: A noisy figure is stated as a range

- **WHEN** a published figure comes from repeated runs whose spread is material
- **THEN** the landing states the median and the observed spread rather than a single value
