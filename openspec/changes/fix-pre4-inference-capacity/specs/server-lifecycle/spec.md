# Spec Delta

## ADDED Requirements

### Requirement: Configured inference preload precedes readiness

The server SHALL complete explicitly configured embedding, script, and image resource preloads, including applicable calibration, before reporting ready. Intentionally lazy paths SHALL remain supported and SHALL expose uninitialized state without being loaded by observation. Readiness documentation SHALL distinguish registration readiness from warm inference readiness.

#### Scenario: Preloaded model calibration
- **WHEN** an explicitly preloaded embedding model is still calibrating
- **THEN** the server SHALL NOT report ready

#### Scenario: Preload failure
- **WHEN** a required configured resource preload fails
- **THEN** startup SHALL report the affected model and failure rather than advertise readiness

#### Scenario: Lazy model remains lazy
- **WHEN** a model has no preload requirement and health or stats are queried
- **THEN** the query SHALL NOT initialize its inference resources
- **AND** its uninitialized serving paths SHALL remain observable

### Requirement: Bounded production overload profile

The production regression profile SHALL configure finite request and connection caps and a finite CPU-admission timeout below its declared application deadline. Overload SHALL reject excess work explicitly, preserve control-command access, and recover after offered load falls. Admission cancellation SHALL NOT destroy resources used by an already-running native call or bypass existing shutdown accounting.

#### Scenario: Overload then recovery
- **WHEN** offered inference load exceeds the configured request cap and later returns to the known sustainable rate
- **THEN** excess work SHALL receive explicit busy outcomes
- **AND** health and inference latency SHALL recover within the profile's declared deadline without a restart

#### Scenario: Shutdown during queued and running work
- **WHEN** shutdown occurs with queued admission waiters and an active native call
- **THEN** canceled queued work SHALL release its queue ownership
- **AND** the native call's resources SHALL remain alive until completion or process termination under the existing shutdown deadline
