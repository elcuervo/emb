## MODIFIED Requirements

### Requirement: Load-time determinism probe

The server SHALL probe batch-determinism at model load whenever batching is enabled, caching the verdict for the model's lifetime. The probe SHALL run before automatic layout calibration, so calibration is told whether the model still batches. A failed probe SHALL disable batching for the model, report the reason, and have calibration measure the single-row path; it SHALL NOT retain worker counts intended only for batching. Output correctness SHALL be preserved.

#### Scenario: Probe runs at load

- **WHEN** a model loads with `batching.timeout > 0`
- **THEN** the server SHALL embed fixed probe texts alone and co-batched and byte-compare the outputs
- **THEN** the verdict SHALL be recorded once and reused for the model's lifetime

#### Scenario: Probe skipped when batching is off

- **WHEN** a model loads with `batching.timeout: 0` (worker pool, never batched)
- **THEN** no probe SHALL run and `batch_determinism` SHALL report `untested`

#### Scenario: Probe failure

- **WHEN** a model's determinism probe fails
- **THEN** it SHALL serve single-row inference with a safe calibrated or fallback pool and a greppable degradation reason

#### Scenario: Probe passes

- **WHEN** a model's probe passes
- **THEN** calibration MAY measure batched candidates and `batch_determinism` SHALL report `passed`
