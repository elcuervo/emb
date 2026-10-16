# Spec Delta

## MODIFIED Requirements

### Requirement: Per-model traffic classification

Each model's recent inference traffic SHALL be classified as `idle`, `latency`, `throughput`, or `saturated`, from per-model in-flight concurrency, dispatch-wait over run-time, and process CPU utilization, sampled at the inference boundary over a rolling window. Reply-cache hits SHALL NOT contribute. The run time and wait time a window is classified on SHALL come from counters that cannot decrease, and SHALL be read as one consistent sample of one model: a window whose run time per run is zero or negative SHALL NOT be classified as `latency`, because a latency classification is what shrinks the allowance.

#### Scenario: Serial traffic is classified latency

- **WHEN** a model serves one evaluation at a time with near-zero dispatch wait
- **THEN** its class SHALL be `latency` while traffic flows, and `idle` when it stops

#### Scenario: Concurrent traffic is classified throughput

- **WHEN** a model's in-flight concurrency exceeds its session count across the sampling window
- **THEN** its class SHALL be `throughput`

#### Scenario: Cache hits do not count as traffic

- **WHEN** every request in the window is served from the reply cache and no inference runs
- **THEN** the class SHALL be `idle` and the run counters SHALL NOT advance

#### Scenario: CPU-bound load is classified saturated

- **WHEN** in-flight concurrency is high and process CPU utilization is at or above the saturation threshold
- **THEN** the class SHALL be `saturated`

#### Scenario: A mis-attributed window cannot shrink the allowance

- **WHEN** a window's run time per run is zero or negative (for example because a wait was subtracted from a run that did not pay it)
- **THEN** the window SHALL NOT be classified as `latency`, and the allowance SHALL NOT be reduced on the strength of it
