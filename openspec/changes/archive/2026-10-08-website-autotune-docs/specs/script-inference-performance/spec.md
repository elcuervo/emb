# Spec Delta

## ADDED Requirements

### Requirement: Explicit session settings win over capacity profiles

An explicitly configured `allow_spinning` SHALL win over the spinning a
`capacity` profile would otherwise pin, and an explicitly configured
`autotune: off` SHALL win over a `capacity` profile that would otherwise leave
the controller active. A profile selects a default layout; it MUST NOT override
a value the operator set.

#### Scenario: Explicit spinning wins over the latency profile

- **WHEN** a model sets `capacity: latency` and `allow_spinning: false`
- **THEN** its scripted sessions SHALL be created with `session.intra_op.allow_spinning` set to `0`, not enabled

#### Scenario: Explicit spinning wins over the throughput profile

- **WHEN** a model sets `capacity: throughput` and `allow_spinning: true`
- **THEN** its scripted sessions SHALL be created with ORT's default spinning, not disabled

#### Scenario: The kill switch wins over the throughput profile

- **WHEN** a model sets `capacity: throughput` and `autotune: off`
- **THEN** the controller SHALL be inactive, the reported `script_autotune_active` SHALL be `0`, and the effective concurrency SHALL stay at the configured value

#### Scenario: Profile defaults still apply when nothing is set

- **WHEN** a model sets `capacity: latency` and neither `allow_spinning` nor `autotune`
- **THEN** its scripted sessions SHALL spin and the controller SHALL be inactive
