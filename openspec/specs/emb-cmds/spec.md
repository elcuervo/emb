# emb-cmds Specification

## Purpose
Specifies the server's EMB.* introspection commands (EMB.INFO, EMB.STATS, EMB.MODELS, EMB.HELP) and the live statistics they expose.

## Requirements

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

### Requirement: MONITOR exposes recent request events

The server SHALL respond to `MONITOR` — named after Redis's MONITOR, but a bounded sequence-query rather than a long-lived stream — with the most recent completed-request events from a bounded, seq-numbered ring buffer. Each event SHALL carry a monotonically increasing sequence number, a unix-microsecond timestamp, the model, the number of texts requested, the latency in microseconds, and an error flag. Request text payloads SHALL NOT be included. Clients SHALL be able to fetch only events after a given sequence number, and the buffer SHALL be bounded (old events may be dropped under sustained load).

Each event SHALL additionally carry the request's attribution: `cached_texts`, the number of its texts the embedding cache answered; and `dispatch_us`, `admit_us` and `run_us`, the time its texts that were *not* cache-answered spent waiting for a session or batch window, waiting at the process CPU admission gate, and running in ORT. The three phases SHALL describe the request's own texts and SHALL NOT exceed its `latency_us`; whatever the latency includes beyond them (protocol, tokenization, cache lookup, encoding) SHALL stay unattributed rather than assigned to a phase. An event whose texts were all cache-answered SHALL report zero for all three phases. Attribution SHALL be collected without allocating on the request path and without adding a clock read to the cache-hit path.

When the connection negotiated RESP3, each event SHALL be a map with the RESP2 field names; under RESP2 each event SHALL be a flat array in the same field order, with the attribution fields appended after `err` so that a client reading the original six fields positionally is unaffected.

#### Scenario: RESP3 reply is an array of maps

- **WHEN** a client that negotiated RESP3 with `HELLO 3` sends `MONITOR` after one completed request
- **THEN** the reply SHALL be an array whose elements are maps keyed `seq`, `at_us`, `model`, `texts`, `latency_us`, `err`, `cached_texts`, `dispatch_us`, `admit_us`, `run_us`

#### Scenario: RESP2 reply is unchanged

- **WHEN** a RESP2 client sends `MONITOR` after one completed request
- **THEN** the reply SHALL be an array of flat arrays whose first six elements are `seq`, `at_us`, `model`, `texts`, `latency_us`, `err` in the order they have today, with `cached_texts`, `dispatch_us`, `admit_us` and `run_us` appended after them

#### Scenario: Empty monitor before any traffic

- **WHEN** client sends `MONITOR` on an idle server
- **THEN** the response SHALL be an empty array

#### Scenario: One event per completed request

- **GIVEN** the server has processed an `EMB minilm "hello"` request
- **WHEN** client sends `MONITOR`
- **THEN** the response SHALL contain exactly one event with the model `minilm`, texts `1`, a positive latency, error `0`, and a positive seq

#### Scenario: Incremental fetch

- **WHEN** client sends `MONITOR <seq>` where `seq` is a previously returned event's sequence
- **THEN** the response SHALL contain only events with sequence greater than `seq`

#### Scenario: Error requests are recorded

- **WHEN** an `EMB` request fails (unknown model, inference error)
- **THEN** its event SHALL carry error `1` and latency measured up to the failure

#### Scenario: Buffer is bounded

- **GIVEN** a sustained load exceeding the ring capacity (8192)
- **WHEN** client sends `MONITOR`
- **THEN** at most 8192 events SHALL be returned, oldest evicted first

#### Scenario: A cache-served request is attributed to the cache

- **WHEN** an `EMB minilm "hello"` request is answered entirely from the reply cache
- **THEN** its event SHALL carry `cached_texts` equal to its `texts`, and `dispatch_us`, `admit_us` and `run_us` all zero

#### Scenario: An uncached request is attributed to its phases

- **WHEN** an `EMB minilm "hello"` request misses the cache and runs
- **THEN** its event SHALL carry `cached_texts` `0`, a `run_us` no greater than its `latency_us`, and the time it waited for a worker or batch window and at the CPU gate in `dispatch_us` and `admit_us`

#### Scenario: A partly cached batch is attributed to both

- **WHEN** an `EMB minilm` request carries several texts of which some are cache hits
- **THEN** its event SHALL carry that many `cached_texts`, and its phases SHALL describe only the texts that ran

#### Scenario: The attribution never exceeds the latency

- **WHEN** an event carries `dispatch_us`, `admit_us` and `run_us`
- **THEN** their sum SHALL NOT exceed its `latency_us`

### Requirement: EMB.MODELS returns a deterministic order

The server SHALL list models from `EMB.MODELS` in a deterministic order — sorted by model name — so repeated calls return the same sequence and clients can render a stable list. Under RESP3 the reply is a map, so order is not significant there.

#### Scenario: Repeated calls agree

- **GIVEN** a server with one or more models loaded
- **WHEN** a RESP2 client sends `EMB.MODELS` twice
- **THEN** both replies list the models in the same name-sorted order

#### Scenario: Models are listed by name

- **GIVEN** models `minilm`, `bge`, and `e5` are registered
- **WHEN** a RESP2 client sends `EMB.MODELS`
- **THEN** the reply lists them in the order `bge`, `e5`, `minilm`
