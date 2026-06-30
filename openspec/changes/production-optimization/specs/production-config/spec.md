## ADDED Requirements

### Requirement: Production configuration

The server SHALL have a recommended production configuration documented for deployment alongside Ruby applications.

#### Scenario: Batching enabled for throughput

- **WHEN** the production config specifies `batching.timeout: 1`
- **THEN** the server SHALL coalesce concurrent requests arriving within 1ms into a single batched ONNX inference

#### Scenario: Thread tuning for Apple Silicon

- **WHEN** the production config specifies `intra_op_threads: 4`
- **THEN** the server SHALL use 4 threads for intra-operator parallelism
