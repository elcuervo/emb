# inference-capacity-autotune Specification

## Purpose

Adapts per-model inference concurrency to the observed traffic shape, so default capacity is near-optimal for serial, bursty, and cache-bound workloads without operator tuning.

## Requirements

### Requirement: Per-model traffic classification

Each model's recent inference traffic SHALL be classified as `idle`, `latency`, `throughput`, or `saturated`, from per-model in-flight concurrency, dispatch-wait over run-time, and process CPU utilization, sampled at the inference boundary over a rolling window. Reply-cache hits SHALL NOT contribute.

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

### Requirement: Adaptive per-session concurrency

The effective concurrent evaluations allowed per named-tensor session SHALL be at most the configured `script_callers_per_session` cap and SHALL adapt within it: growing toward observed demand while sessions are busy and CPU has headroom, and shrinking after sustained low demand. Explicit configuration SHALL set the cap.

#### Scenario: Concurrency grows under burst

- **WHEN** in-flight reaches the current allowance while CPU headroom exists and dispatch wait is positive
- **THEN** the allowance SHALL increase, up to the configured cap

#### Scenario: Concurrency shrinks when the burst ends

- **WHEN** dispatch wait stays near zero for the configured quiet window
- **THEN** the allowance SHALL decrease toward one

#### Scenario: The configured cap is never exceeded

- **WHEN** `script_callers_per_session` is N
- **THEN** the effective allowance SHALL never exceed N

#### Scenario: Adaptation is damped

- **WHEN** the traffic class alternates across consecutive windows
- **THEN** the allowance SHALL NOT change on every window

### Requirement: CPU-saturation safety gate

The server SHALL NOT increase concurrency while process CPU utilization is at or above the saturation threshold. It SHALL instead record a provisioning recommendation naming the model and the affected thread or session settings.

#### Scenario: No growth while saturated

- **WHEN** CPU is saturated and the class is `saturated`
- **THEN** the concurrency allowance SHALL NOT increase

#### Scenario: A provisioning recommendation is recorded

- **WHEN** saturation persists across a sampling window
- **THEN** a recommendation to raise `script_workers` or `intra_op_threads` SHALL be recorded once per model per transition

### Requirement: Capacity profiles

Each model SHALL accept `capacity: auto|latency|throughput` (default `auto`). `latency` SHALL create sessions with ORT spinning enabled and one caller per session; `throughput` SHALL create shared sessions with spinning disabled; `auto` SHALL keep the derived layout and enable adaptation.

#### Scenario: Latency profile pins the low-latency layout

- **WHEN** `capacity: latency` is configured
- **THEN** the model's sessions SHALL be created with spinning enabled and an initial concurrency of one

#### Scenario: Throughput profile pins the shared layout

- **WHEN** `capacity: throughput` is configured
- **THEN** the model's sessions SHALL be created with spinning disabled and a shared concurrency allowance

#### Scenario: Auto is the default

- **WHEN** `capacity` is unset
- **THEN** the derived layout SHALL apply and the runtime controller SHALL be active

### Requirement: Autotune controls and limits

Each model SHALL accept `autotune: off|callers|auto` (default `auto`). `off` SHALL keep concurrency fixed at the configured value. Adaptation SHALL apply only to named-tensor script sessions and SHALL NOT change the session count or per-session thread count at runtime; embedding and image pools SHALL keep their load-time concurrency.

#### Scenario: Off keeps concurrency fixed

- **WHEN** `autotune: off` is configured
- **THEN** the effective concurrency SHALL equal the configured value for the process lifetime

#### Scenario: No runtime reshape

- **WHEN** the controller adapts
- **THEN** the session count and per-session thread count SHALL be unchanged

#### Scenario: Embedding and image pools are unchanged

- **WHEN** a model serves embedding or image traffic
- **THEN** its pool's concurrency SHALL remain the load-time value
