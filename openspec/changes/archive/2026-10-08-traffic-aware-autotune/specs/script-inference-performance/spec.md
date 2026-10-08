# Spec Delta

## MODIFIED Requirements

### Requirement: Shared script sessions with optional spinning

A model SHALL accept `script_callers_per_session` as a per-session concurrency cap (default 4). Up to that many evaluations SHALL run concurrently on each named-tensor session; within the cap the effective allowance SHALL be governed by `inference-capacity-autotune`. Concurrent runs on one session SHALL return the same outputs as serial runs. A model SHALL accept `allow_spinning`, which maps to ORT `session.intra_op.allow_spinning`. When unset, scripted sessions SHALL spin off while shared (`script_callers_per_session > 1` or `capacity: throughput`) and keep ORT's default otherwise; an explicit value SHALL always win.

#### Scenario: Concurrent runs on one session are correct

- **GIVEN** a model with `script_workers: 1` and `script_callers_per_session: 4`
- **WHEN** four evaluations with different texts run concurrently
- **THEN** all four SHALL run without waiting for each other, and each reply SHALL equal its serial reply byte for byte

#### Scenario: Spinning disabled

- **WHEN** a model sets `allow_spinning: false`
- **THEN** its sessions SHALL be created with `session.intra_op.allow_spinning` set to `0`

#### Scenario: Shared sessions default to spinning off

- **WHEN** a model leaves `allow_spinning` unset and `script_callers_per_session` unset or above 1
- **THEN** its scripted sessions SHALL be created with `session.intra_op.allow_spinning` set to `0`

#### Scenario: A single caller keeps ORT's spinning

- **WHEN** a model sets `script_callers_per_session: 1` and leaves `allow_spinning` unset
- **THEN** its scripted sessions SHALL keep ORT's default `allow_spinning`

#### Scenario: The cap bounds adaptive concurrency

- **WHEN** `script_callers_per_session: 4` is configured and the controller raises concurrency under load
- **THEN** at most four evaluations SHALL run concurrently on any one session
