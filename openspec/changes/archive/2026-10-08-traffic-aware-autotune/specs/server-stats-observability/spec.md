# Spec Delta

## ADDED Requirements

### Requirement: Autotune state is observable

`EMB.INFO <model>` and the per-model lines of `EMB.STATS` SHALL report, for the model's scripted path, the current traffic class, in-flight inference count, current and target concurrency allowance, and whether autotune is active. Field reads SHALL be atomic, add no request-path allocation, and the `EMB.STATS` RESP array count SHALL still match the emitted fields.

#### Scenario: Autotune fields are present

- **WHEN** `EMB.INFO <model>` is read for a model serving scripted traffic
- **THEN** the reply SHALL include `script_traffic_class`, `script_inflight`, `script_concurrency_current`, `script_concurrency_target`, and `script_autotune_active`

#### Scenario: RESP array count still matches

- **WHEN** `EMB.STATS` is parsed after the autotune fields are added
- **THEN** the declared RESP array count SHALL equal the number of elements emitted
