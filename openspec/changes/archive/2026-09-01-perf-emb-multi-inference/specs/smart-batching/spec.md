# smart-batching Specification (delta)

## ADDED Requirements

### Requirement: Idle-flush for single-request latency

When the batching run loop is idle (no batch currently in flight), a newly arrived request
SHALL be executed immediately rather than waiting out the batching window, so a lone
request does not incur an artificial delay beyond configuration when the server is idle;
under concurrent load the window SHALL still coalesce requests as configured.

#### Scenario: Lone request not delayed when run loop idle

- **WHEN** the server is idle and a single embedding request arrives for a batched model
- **THEN** the request SHALL be served without waiting the configured `timeout`
- **AND** the returned embedding SHALL be correct and byte-identical to the same text via a
  non-batched path

#### Scenario: Bursts still batch under load

- **WHEN** many concurrent requests arrive for the same batched model
- **THEN** consecutive requests SHALL still be coalesced into shared ONNX runs within the
  configured window and token budget
