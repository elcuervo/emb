## ADDED Requirements

### Requirement: MONITOR exposes recent request events

The server SHALL respond to `MONITOR` — named after Redis's MONITOR, but a bounded sequence-query rather than a long-lived stream — with the most recent completed-request events from a bounded, seq-numbered ring buffer. Each event SHALL carry a monotonically increasing sequence number, a unix-microsecond timestamp, the model, the number of texts requested, the latency in microseconds, and an error flag. Request text payloads SHALL NOT be included. Clients SHALL be able to fetch only events after a given sequence number, and the buffer SHALL be bounded (old events may be dropped under sustained load).

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