## ADDED Requirements

### Requirement: Per-model dispatch and run timings

`EMB.STATS` and `EMB.INFO` SHALL report the following per model, separately for the script path and the embedding path:

- cumulative `dispatch_wait_us`: time spent waiting for a session or worker
- cumulative `run_us`: ORT run time
- `runs`
- `sessions_busy` and `sessions_total`

Counter reads SHALL be atomic. Collecting the counters SHALL NOT allocate on the request path. The RESP array count SHALL match the emitted fields.

#### Scenario: Serial traffic has no dispatch wait

- **WHEN** scripted evaluations are sent one at a time to a model with idle sessions
- **THEN** the increase in `dispatch_wait_us` divided by the increase in `runs` SHALL be near zero, and `run_us` SHALL increase

#### Scenario: Contention is visible

- **WHEN** concurrency exceeds the model's session count
- **THEN** `dispatch_wait_us` SHALL increase, and `sessions_busy` SHALL equal `sessions_total` while all sessions are running

#### Scenario: RESP array count still matches

- **WHEN** `EMB.STATS` is parsed after the new fields are added
- **THEN** the declared RESP array count SHALL equal the number of elements emitted
