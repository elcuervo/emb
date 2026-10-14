# Spec Delta

## Purpose

Establishes which hosts may produce trustworthy performance evidence, records every measured run as a comparable bundle, and defines the two-tier reference performance test — a per-change smoke gate and a per-host reference run — that keeps the autotune behaviour and the scripted-inference throughput from regressing.

## ADDED Requirements

### Requirement: Validation host contract

Before any measurement, the validation tooling SHALL determine whether a host may produce reference performance evidence, reporting its architecture, effective CPU allowance and the source of that allowance, memory limit, and sampled CPU steal and throttle deltas, together with the server's own detected budget. It SHALL return a qualified or unqualified verdict and SHALL name the property that failed when unqualified.

#### Scenario: Quota below the presented core count

- **WHEN** a host's container CPU quota allows appreciably less CPU than the host presents as available (a shared or burstable plan)
- **THEN** the verdict SHALL be unqualified
- **AND** the report SHALL name the CPU quota as the failed property, and SHALL report the budget the server derived from that quota rather than the presented core count

#### Scenario: Dedicated host qualifies

- **WHEN** the effective CPU allowance matches the presented CPUs, or no quota is enforced, and sampled steal stays below the configured threshold
- **THEN** the verdict SHALL be qualified

#### Scenario: No cgroup CPU file

- **WHEN** the host exposes no cgroup CPU quota
- **THEN** the report SHALL state that the allowance comes from the scheduler and affinity limits
- **AND** it SHALL NOT report a quota

#### Scenario: Preflight guards a run

- **WHEN** a recorded run starts on an unqualified host
- **THEN** the run SHALL be marked non-gold and the failed property SHALL be recorded in its bundle

### Requirement: Recorded evidence bundles

Every recorded performance run SHALL emit one bundle naming the host fingerprint, the revision, the model inputs, the configuration, the measured cells, and the outcome of every assertion it ran, and the diff tooling SHALL refuse to compare bundles whose host fingerprints differ.

#### Scenario: Bundle identity

- **WHEN** a recorded run completes
- **THEN** its bundle SHALL name the architecture, effective CPU allowance and its source, revision, model digests, and configuration

#### Scenario: Unmet precondition is not a pass

- **WHEN** a phase that must reach a condition (for example CPU saturation) does not reach it
- **THEN** the assertion SHALL be recorded as invalid rather than passed
- **AND** the bundle SHALL NOT be usable as a reference

#### Scenario: Cross-host comparison refused

- **WHEN** two bundles with different host fingerprints are compared
- **THEN** the tooling SHALL refuse and print both fingerprints

#### Scenario: Same-host comparison accepted

- **WHEN** the fingerprints match
- **THEN** the diff SHALL apply the configured tolerances per cell and print a per-cell verdict

### Requirement: Reference performance test

A recorded reference run SHALL reproduce the autotune behaviour contract on a host that satisfies the validation host contract — traffic class sequence, concurrency allowance transitions and its cap, the disable switch, the capacity profiles, reply identity, and the reported autotune state — and SHALL gate its throughput and latency cells against a baseline recorded on the same host.

#### Scenario: Behaviour contract reproduced

- **WHEN** serial, then burst, then serial traffic is driven against a scripted model
- **THEN** the classes SHALL be `latency` (then `idle` when traffic stops), then `throughput` or `saturated`, then `latency` again
- **AND** the allowance SHALL halve after the sustained quiet window and return to the cap on the first `throughput` window

#### Scenario: Same-host tolerance gate

- **WHEN** a changed build's per-cell throughput falls more than the recorded tolerance below the same-host baseline
- **THEN** the run SHALL fail, naming the cells that regressed

#### Scenario: Absolute numbers never gate

- **WHEN** a baseline was recorded on a different host
- **THEN** the tooling SHALL refuse to use it as a tolerance reference

#### Scenario: Adaptation does not reshape

- **WHEN** the allowance adapts
- **THEN** the recorded session count and per-session thread count SHALL be unchanged

### Requirement: Per-change smoke gate

The repository SHALL provide a smoke gate that exercises the autotune path end to end on an in-repo fixture without network model downloads and within the existing test job's wall-clock budget, asserting the class sequence, the allowance transitions and its cap, the disable switch, profile pinning, cache-hit idleness, layout immutability, reply identity, and a bounded relative throughput change against a fixed-concurrency control arm measured in the same run.

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

### Requirement: Gate self-verification

The gate SHALL be verified to fail on planted regressions before it is relied on, covering at least an inverted allowance dwell, a saturation threshold that cannot be reached, reply-cache hits counted as traffic, and a runtime change to the session or thread layout, and each failure SHALL name the assertion that detected it.

#### Scenario: Planted regression is detected

- **WHEN** any listed regression is applied to the tree
- **THEN** the gate SHALL fail and name the corresponding assertion

#### Scenario: Unmodified tree passes

- **WHEN** the tree is unmodified
- **THEN** the gate SHALL pass, reporting only the documented host-dependent skips
