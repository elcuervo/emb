## Requirements

### Requirement: Batch concurrent requests

The server SHALL collect concurrent embedding requests for the same model and execute them as a single batched ONNX inference.

#### Scenario: Multiple concurrent EMBs batched together

- **WHEN** multiple clients send `EMB` commands to the same model within the batching window
- **THEN** the server SHALL collect the texts and run a single ONNX `Run()` with batch_size = number of texts
- **THEN** each client SHALL receive its correct embedding vector

#### Scenario: Single EMB within window

- **WHEN** a single `EMB` request arrives and no other requests arrive within the batching window
- **THEN** the server SHALL execute after the window expires (acknowledging the latency tradeoff)

#### Scenario: Configurable batching timeout

- **WHEN** a model config has `timeout: 5`
- **THEN** the server SHALL wait at most 5ms before executing the batched inference
- **THEN** the default timeout SHALL be 2ms when unset

#### Scenario: Max batch size

- **WHEN** the number of accumulated requests reaches the max batch size (default 32)
- **THEN** the server SHALL execute immediately without waiting for the timeout

### Requirement: Throughput improvement

The smart batcher SHALL improve throughput under concurrent load without degrading single-request latency by more than the configured timeout.

#### Scenario: Concurrent throughput

- **WHEN** 8 concurrent clients send requests
- **THEN** throughput SHALL exceed the current baseline (509 req/s) by at least 50%

#### Scenario: Single-request latency

- **WHEN** a single client sends requests
- **THEN** p50 latency SHALL NOT exceed the baseline (3ms) + timeout
