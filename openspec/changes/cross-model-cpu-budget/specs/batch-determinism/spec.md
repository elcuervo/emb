## MODIFIED Requirements

### Requirement: Determinism probe precedes calibration

The server SHALL run the batch determinism probe before automatic layout calibration. A failed probe SHALL disable batching for that model, report the reason, and calibrate its single-row path. It SHALL preserve output correctness and SHALL NOT retain worker counts intended only for batching.

#### Scenario: Probe failure

- **WHEN** a model's determinism probe fails
- **THEN** it SHALL serve single-row inference with a safe calibrated or fallback pool and a greppable degradation reason

#### Scenario: Probe passes

- **WHEN** a model's probe passes
- **THEN** calibration MAY measure batched candidates and batch_determinism SHALL report passed

