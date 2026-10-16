# Spec Delta

## MODIFIED Requirements

### Requirement: Per-change smoke gate
The repository SHALL provide a smoke gate that exercises the autotune path end to end on an in-repo fixture without network model downloads and within the existing test job's wall-clock budget, asserting the class sequence, the allowance transitions and its cap, the disable switch, profile pinning, cache-hit idleness, layout immutability, reply identity, and a bounded relative throughput change against a fixed-concurrency control arm measured in the same run. The gate SHALL emit one machine-readable summary per arm, each naming the arm it summarizes and reporting its own pass, fail and skipped counts, and a summary SHALL NOT count an assertion that did not run as a pass.

#### Scenario: Gate is cheap and offline

- **WHEN** the gate runs
- **THEN** it SHALL complete inside the test job's wall-clock budget without downloading a model and without Docker

#### Scenario: Control-arm relative gate

- **WHEN** the adaptive arm sustains less throughput than the fixed-concurrency control arm beyond the configured tolerance in the same run
- **THEN** the gate SHALL fail
- **WHEN** the difference is within tolerance
- **THEN** the gate SHALL pass

#### Scenario: Kill switch and profiles

- **WHEN** autotune is disabled, or a capacity profile is configured
- **THEN** the gate SHALL assert concurrency stays at the configured value, the profile's layout applies, and the reported autotune state matches

#### Scenario: Cache hits are idle

- **WHEN** a sampling window is served entirely from the reply cache
- **THEN** the class SHALL be `idle` and the run counters SHALL NOT advance

#### Scenario: Allowance bounds and damping

- **WHEN** the allowance grows or shrinks
- **THEN** it SHALL never exceed the configured cap and never fall below one
- **AND** it SHALL NOT change on every sampling window

#### Scenario: Replies are identical across arms

- **WHEN** the same inputs are served under different profiles and switches
- **THEN** the replies SHALL be byte-identical

#### Scenario: Unreachable assertion is reported

- **WHEN** the host cannot reach the saturation threshold
- **THEN** the gate SHALL report that assertion as skipped, naming the host property that prevented it
- **AND** it SHALL NOT report a pass for it

#### Scenario: An arm that did not run is not a green gate

- **WHEN** a check requires proof that the gate ran, and one arm produced no summary at all while another arm printed one
- **THEN** the check SHALL fail, because it SHALL read the summary of the arm it is checking rather than any summary the run happened to print

### Requirement: Gate self-verification
The gate SHALL be verified to fail on planted regressions before it is relied on, covering at least an inverted allowance dwell, a saturation threshold that cannot be reached, reply-cache hits counted as traffic, and a runtime change to the session or thread layout, and each failure SHALL name the assertion that detected it. The sweep SHALL run against the tree that is being released: every planted patch SHALL apply to that tree's revision, and the sweep SHALL be a required check rather than a documented manual step, so a patch that has drifted out of applicability fails the sweep instead of silently stopping the verification.

#### Scenario: Planted regression is detected

- **WHEN** any listed regression is applied to the tree
- **THEN** the gate SHALL fail and name the corresponding assertion

#### Scenario: Unmodified tree passes

- **WHEN** the tree is unmodified
- **THEN** the gate SHALL pass, reporting only the documented host-dependent skips

#### Scenario: A stale mutant fails the sweep

- **WHEN** a planted patch no longer applies to the revision under test
- **THEN** the sweep SHALL fail and name that patch, so the sweep cannot report success while a regression it claims to plant was never applied

#### Scenario: The sweep runs where the gate runs

- **WHEN** a pull request changes the gate, the admission path, the autotuner, or the scripted runtime
- **THEN** the sweep SHALL be executed as part of the change's required checks
