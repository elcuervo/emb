# emb-cmds Specification (delta)

## MODIFIED Requirements

### Requirement: EMB.INFO shows real model statistics

The server SHALL respond to `EMB.INFO <model>` with real request count and average latency from internal counters. When the connection negotiated RESP3, the reply SHALL be a map; under RESP2 it SHALL remain the flat alternating key/value array. Field names SHALL be identical in both encodings.

#### Scenario: Model info shows request count

- **WHEN** client sends `EMB.INFO minilm` after 10 embedding requests
- **THEN** response includes `requests: 10` (not the hardcoded 0)

#### Scenario: Model info shows average latency

- **WHEN** client sends `EMB.INFO minilm`
- **THEN** response includes `avg_latency_us` with the cumulative average latency per request in microseconds

#### Scenario: RESP3 reply is a map

- **GIVEN** a connection that negotiated RESP3
- **WHEN** client sends `EMB.INFO minilm`
- **THEN** the reply SHALL be a RESP3 map with the same keys as the RESP2 flat-pair reply

### Requirement: EMB.STATS shows per-model breakdown

The server SHALL respond to `EMB.STATS` with per-model request counts in addition to server-wide totals. When the connection negotiated RESP3, the reply SHALL be a map; under RESP2 it SHALL remain the flat alternating key/value array.

#### Scenario: Server stats includes per-model data

- **WHEN** client sends `EMB.STATS` with 2 loaded models
- **THEN** response includes per-model request counts alongside uptime and total requests

#### Scenario: RESP3 reply is a map

- **GIVEN** a connection that negotiated RESP3
- **WHEN** client sends `EMB.STATS`
- **THEN** the reply SHALL be a RESP3 map whose keys equal the RESP2 flat-pair field names (and nested per-model data keeps its shape)

### Requirement: EMB.MODELS lists models

The server SHALL respond to `EMB.MODELS` by listing available models. When the connection negotiated RESP3, the reply SHALL be a map keyed by model name whose values carry the same fields as today's per-model arrays; under RESP2 it SHALL remain the array-of-arrays reply.

#### Scenario: RESP3 reply is a model map

- **GIVEN** a connection that negotiated RESP3 with models `minilm` and `e5` registered
- **WHEN** client sends `EMB.MODELS`
- **THEN** the reply SHALL be a RESP3 map with keys `minilm` and `e5`
- **AND** each value SHALL contain the model's dim and status