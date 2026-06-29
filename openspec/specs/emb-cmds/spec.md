## ADDED Requirements

### Requirement: EMB.INFO shows real model statistics

The server SHALL respond to `EMB.INFO <model>` with real request count and average latency from internal counters.

#### Scenario: Model info shows request count

- **WHEN** client sends `EMB.INFO minilm` after 10 embedding requests
- **THEN** response includes `requests: 10` (not the hardcoded 0)

#### Scenario: Model info shows average latency

- **WHEN** client sends `EMB.INFO minilm`
- **THEN** response includes `avg_latency_us` with the cumulative average latency per request in microseconds

### Requirement: EMB.STATS shows per-model breakdown

The server SHALL respond to `EMB.STATS` with per-model request counts in addition to server-wide totals.

#### Scenario: Server stats includes per-model data

- **WHEN** client sends `EMB.STATS` with 2 loaded models
- **THEN** response includes per-model request counts alongside uptime and total requests
