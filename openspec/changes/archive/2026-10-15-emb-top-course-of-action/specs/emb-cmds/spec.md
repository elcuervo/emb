# Spec Delta

## MODIFIED Requirements

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
