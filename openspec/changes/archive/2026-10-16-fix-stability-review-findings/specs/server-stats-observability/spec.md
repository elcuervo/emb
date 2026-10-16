# Spec Delta

## MODIFIED Requirements

### Requirement: Per-model dispatch and run timings

`EMB.STATS` and `EMB.INFO` SHALL report the following per model, separately for the script path and the embedding path:

- cumulative `dispatch_wait_us`: time spent waiting for a session or worker
- cumulative `admission_wait_us`: time spent waiting at the process CPU admission gate, kept out of `run_us` and out of `dispatch_wait_us`
- cumulative `run_us`: ORT run time
- `runs`
- `sessions_busy` and `sessions_total`

Counter reads SHALL be atomic. Collecting the counters SHALL NOT allocate on the request path. The RESP array count SHALL match the emitted fields.

The attribution SHALL be per run, not per session: the wait a run paid at the gate SHALL travel with that run — returned to the caller that paid it — and SHALL NOT be read from a slot several callers of one session share, since a concurrent run can overwrite it. A request SHALL account for every text it ran, not only the last. The per-model counters SHALL be monotonic: `run_us` and `admission_wait_us` SHALL NOT decrease over the life of the process, and no counter SHALL account for a wait a different caller paid.

The same three phases SHALL be attributable to the individual completed request that paid them, rather than only to the model's cumulative totals: a `MONITOR` event SHALL carry the request's own `dispatch_us`, `admit_us` and `run_us`, measured where they are incurred — by the pool or batcher that handed the request to a session, and by the run itself — and carried out on the response value rather than reconstructed by the reader. A request answered entirely from the reply cache SHALL be attributable to the cache path, and the attribution SHALL be collected for every inference command that records an event, not only `EMB`.

#### Scenario: Serial traffic has no dispatch wait

- **WHEN** scripted evaluations are sent one at a time to a model with idle sessions
- **THEN** the increase in `dispatch_wait_us` divided by the increase in `runs` SHALL be near zero, and `run_us` SHALL increase

#### Scenario: Contention is visible

- **WHEN** concurrency exceeds the model's session count
- **THEN** `dispatch_wait_us` SHALL increase, and `sessions_busy` SHALL equal `sessions_total` while all sessions are running

#### Scenario: Gate wait is counted apart from run time

- **WHEN** a model's runs wait at the CPU admission gate
- **THEN** `admission_wait_us` SHALL increase while `run_us` accounts only for the run, and the two SHALL be reported separately from `dispatch_wait_us`

#### Scenario: Counters never run backwards under shared sessions

- **WHEN** callers run concurrently on a session leased to more than one caller, some of them waiting at the gate
- **THEN** `run_us` SHALL NOT decrease between two reads, and the wait each caller paid SHALL be attributed to that caller rather than to whichever run reads the slot first

#### Scenario: Every text of a request is attributed

- **WHEN** one `EMB`/`EMB.MULTI` request carries several texts that each run and each wait at the gate
- **THEN** the request SHALL account for the claims of all of them, so `run_us` reflects every run and `admission_wait_us` every wait it paid

#### Scenario: A completed request carries its own phases

- **WHEN** an `EMB` request misses the cache, waits for a worker and runs
- **THEN** its `MONITOR` event SHALL carry that request's own `dispatch_us`, `admit_us` and `run_us`, each measured where it was incurred

#### Scenario: RESP array count still matches

- **WHEN** `EMB.STATS` is parsed after the new fields are added
- **THEN** the declared RESP array count SHALL equal the number of elements emitted
