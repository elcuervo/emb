# server-stats-observability delta

## MODIFIED Requirements

### Requirement: EMB.STATS reports live active requests

The server SHALL report the number of inference requests currently being processed in `EMB.STATS` as the integer field `active_requests`, replacing the previously hardcoded `"0"`. The counted commands SHALL be `EMB`, `EMB.MULTI`, `EMB.IMG`, `EMB.IMGMULTI`, `EMB.EVAL`, and `EMB.EVSHA`.

#### Scenario: Active requests during in-flight work

- **GIVEN** N `EMB` requests are being processed concurrently
- **WHEN** `EMB.STATS` is called
- **THEN** `active_requests` SHALL report N (or the capped value under `max_concurrent_requests`)

#### Scenario: Idle server reports zero

- **WHEN** no inference requests are in flight
- **THEN** `active_requests` SHALL report `0`

#### Scenario: Scripted evaluations count as active requests

- **GIVEN** a long-running `EMB.EVSHA` evaluation is in flight
- **WHEN** `EMB.STATS` is called from another connection
- **THEN** `active_requests` SHALL include it

## ADDED Requirements

### Requirement: Scripted evaluations are observable

Scripted evaluations SHALL be first-class in the server's observability surface, so operators can see them alongside native embedding traffic rather than only through `max_concurrent_requests` behaviour.

- Every completed `EMB.EVAL`/`EMB.EVSHA` evaluation SHALL append a `MONITOR` event with the model, the number of texts, the latency in microseconds, and an error flag. Request text payloads SHALL NOT be recorded, exactly as for `EMB`.
- `EMB.STATS` SHALL report cumulative scripted counters: `script_requests`, `script_errors`, and `script_avg_latency_us`.
- Per-model breakdowns in `EMB.STATS`/`EMB.INFO` SHALL include scripted counts for models that have served scripted traffic, distinct from embedding counts.
- Stats SHALL expose the scripted resource footprint per model: the number of named-tensor sessions open and whether the script tokenizer is loaded, so the memory cost of scripting is inspectable without reading process memory.
- Counter reads SHALL be atomic and SHALL NOT allocate unboundedly or scan caches.

#### Scenario: Scripted request appears in MONITOR

- **WHEN** a client sends `EMB.EVSHA` and then queries `MONITOR`
- **THEN** the event ring contains an event for that evaluation with the model, text count, latency, and error flag, and no text payload

#### Scenario: Scripted counters are cumulative

- **WHEN** N scripted evaluations complete, one of which fails
- **THEN** `script_requests` is N, `script_errors` is 1, and `script_avg_latency_us` reflects the completed evaluations

#### Scenario: Per-model scripted counts are distinguishable

- **WHEN** a model has served both `EMB` requests and scripted evaluations
- **THEN** its per-model entry reports the embedding request count and the scripted request count separately

#### Scenario: Script resource footprint is reported

- **WHEN** `EMB.EVSHA` runs a script that uses `emb.run`
- **THEN** the model's reported script session count is at least 1, and it is 0 for a model that has only run `emb.embed` scripts

#### Scenario: RESP array count still matches

- **WHEN** `EMB.STATS` is parsed after the new fields are added
- **THEN** the declared RESP array count equals the number of elements emitted, with and without scripted traffic
