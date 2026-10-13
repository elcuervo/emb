# Spec Delta

## ADDED Requirements

### Requirement: Production-policy release comparison

The harness SHALL drive known-good, known-bad pre4, and candidate release artifacts through their production startup/configuration paths using one external driver and identical pinned workload inputs. It SHALL record artifact and input digests and server-observed resource constraints. The driver SHALL NOT be included in measured server CPU consumption.

#### Scenario: Governor enabled in candidate
- **WHEN** the candidate runs automatic CPU planning
- **THEN** the harness SHALL verify the enabled governor and actual selected layouts before accepting measured evidence

#### Scenario: Known-bad discrimination
- **WHEN** the incident reproduction is accepted as a regression gate
- **THEN** known-good SHALL pass, pre4 SHALL fail the intended service assertion, and the candidate SHALL pass the same unchanged assertion
- **AND** an unrelated startup failure, missing model, or malformed configuration SHALL NOT count as reproducing the regression

### Requirement: Complete offered-work accounting

The harness SHALL schedule offered work independently of response completion and classify every pair exactly once as successful, errored, timed out, or dropped before start. It SHALL validate successful embedding replies and correctness, apply bounded request/drain deadlines, measure throughput using elapsed wall time, and report latency from scheduled time separately from wire-start time.

#### Scenario: RESP error response
- **WHEN** a server returns a complete RESP error instead of an embedding
- **THEN** the pair SHALL be classified as an error rather than a successful completion

#### Scenario: Scheduling overload
- **WHEN** a scheduled pair cannot start or complete within its declared limits
- **THEN** it SHALL remain visible as a drop or timeout in offered-work accounting

#### Scenario: Warm-cache case
- **WHEN** a warm-cache phase is measured
- **THEN** its corpus SHALL be explicitly warmed beforehand and actual inference counters SHALL distinguish cache hits from native work

### Requirement: Deployment-shaped regression phases

Required regression profiles SHALL cover cold and preloaded startup, one-hot traffic with an idle configured peer, serial and parallel paired traffic, mixed text lengths and cache states, overload and recovery. The matrix SHALL include actual incident limits, fractional-bandwidth controls, and both batched and determinism-degraded paths. Production deadline inputs SHALL be declared before measurement.

#### Scenario: Idle peer capacity
- **WHEN** one model receives sustained load while its peer remains idle
- **THEN** useful throughput, tail latency, selected layout and admission pressure SHALL be compared with known-good under identical resource constraints

#### Scenario: Constrained reproduction host
- **WHEN** a host intentionally uses the incident's fractional quota or burstable constraints
- **THEN** reference-host disqualification SHALL NOT skip the constrained reproduction
- **AND** the bundle SHALL label that evidence as constrained reproduction rather than a qualified reference baseline

#### Scenario: Overload and recovery
- **WHEN** a controlled burst ends
- **THEN** the profile SHALL enforce its declared health and recovery deadlines, bound resident memory, and reject OOM or restart

### Requirement: Statistically supported release verdict

Comparative gates SHALL use at least three same-host interleaved rounds and a good-versus-good noise check. Default thresholds SHALL require at least 95 percent known-good useful throughput and at most 110 percent known-good pair p50, p95 and p99. Each gated steady-state tail cell SHALL contain at least 1000 successful pairs and run at least 120 seconds. Insufficient or noisy evidence SHALL be inconclusive, never passing.

#### Scenario: Sustainable offered rate
- **WHEN** the profile runs at the known-good sustainable rate
- **THEN** unexpected errors, timeouts, drops or growing backlog SHALL fail acceptance regardless of successful-request latency

#### Scenario: Zero control throughput
- **WHEN** a control arm completes no valid inference
- **THEN** relative-rate comparison SHALL fail as invalid rather than pass against a zero floor

#### Scenario: Noisy control comparison
- **WHEN** repeated good-versus-good runs exceed declared tolerances
- **THEN** the result SHALL be inconclusive and thresholds SHALL NOT be widened automatically

### Requirement: Fail-closed regression evidence

The versioned result bundle SHALL identify artifact, model, tokenizer, configuration, corpus, host/runtime and server resource provenance, with phase counts, correctness, latency, CPU, memory, throttle, admission, startup and recovery evidence. Comparison SHALL reject missing or duplicate required cells, mismatched inputs, invalid required assertions, incomplete runs and unapproved baselines before applying numeric tolerances.

#### Scenario: Empty or incomplete bundle
- **WHEN** a bundle has no required cells or omits a required phase
- **THEN** the comparator SHALL fail explicitly rather than emit a successful empty comparison

#### Scenario: Different model or configuration
- **WHEN** arms differ in a required model, tokenizer, configuration or corpus digest
- **THEN** the comparator SHALL refuse performance equivalence and name the mismatch

#### Scenario: Older release lacks a diagnostic field
- **WHEN** an older artifact lacks a newer admission metric
- **THEN** the field SHALL be recorded unavailable without inventing a value
- **AND** mandatory external correctness, latency and availability assertions SHALL still be enforced

### Requirement: Enforced release regression gate

Publishing release artifacts SHALL depend on a passing production-shaped regression job, including the incident profile and known-bad discrimination. Relevant inference-policy changes SHALL also run a production-entrypoint smoke and measured regression coverage in CI. Missing fixtures, skipped required phases or unavailable execution infrastructure SHALL NOT be treated as a pass. Failed evidence SHALL be retained.

#### Scenario: Required job missing or invalid
- **WHEN** release regression evidence is absent, invalid or skipped
- **THEN** artifact publication SHALL remain blocked

#### Scenario: Harness self-verification
- **WHEN** the harness is exercised with application-error replies, an empty bundle, zero successful control work, or deliberate throughput collapse
- **THEN** it SHALL fail the corresponding named assertion
- **AND** a valid good-versus-good comparison SHALL pass
