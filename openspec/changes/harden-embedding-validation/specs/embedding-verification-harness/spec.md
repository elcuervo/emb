## Purpose

Provides the shared, testable verification harness the project uses to compare
served embeddings against an independent reference: one RESP client, a small
set of verifier commands built on it, and the `just`/CI gates that keep the
harness itself covered and free of unreachable code.

## ADDED Requirements

### Requirement: Single shared RESP verification client

All embedding verifier commands SHALL use one shared RESP client implementation rather than each defining their own wire reader.

#### Scenario: Bulk and array replies

- **WHEN** a verifier sends an embed command and the server replies with a bulk string
- **THEN** the client returns the decoded float32 vector for that reply
- **WHEN** the server replies with an array (including null elements)
- **THEN** the client returns the array in order, with nulls preserved as nulls

#### Scenario: Server error reply

- **WHEN** the server replies with a Redis error (unknown model, authentication, bad arity)
- **THEN** the client returns an error carrying the server's message rather than attempting to decode a vector

#### Scenario: Connection and read timeouts

- **WHEN** the server is unreachable or stops responding mid-reply
- **THEN** the client fails within a configured timeout with an error naming the address, instead of blocking indefinitely

#### Scenario: Client is testable without a server

- **WHEN** the harness unit tests run
- **THEN** the client's framing, decoding, timeout, and error handling are exercised against an in-process responder with no live server and no ONNX model

### Requirement: Verifier commands are configurable and fail clearly

Each verifier command SHALL take its server address, model names, dimension, and corpus from flags or environment rather than hardcoded constants, and SHALL exit non-zero with a named cause when a precondition is not met.

#### Scenario: Inputs are configurable

- **WHEN** a user runs a verifier with an explicit address, model, and dimension
- **THEN** the verifier uses those values instead of compiled-in defaults

#### Scenario: Missing model or reference

- **WHEN** a verifier is pointed at a model the server has not loaded, or at a reference artifact it cannot read
- **THEN** it exits non-zero with an error naming the missing model or artifact

### Requirement: EMB.MULTI byte-equality verification is reproducible

The EMB.MULTI verifier SHALL compare the bytes returned by `EMB.MULTI` against the bytes returned by sequential `EMB` calls for the same pairs, and SHALL run from a model artifact that the documented model-download step actually produces.

#### Scenario: Byte-identical results

- **WHEN** the EMB.MULTI verifier runs against a server with two configured models
- **THEN** each `EMB.MULTI` element is byte-identical to the corresponding sequential `EMB` reply, for both cross-model and same-model pair sets

#### Scenario: Mismatch is named

- **WHEN** an element differs from the sequential reply
- **THEN** the verifier exits non-zero naming the model and text of the differing element

#### Scenario: Downloads the artifact the config uses

- **WHEN** the verifier obtains its model using the documented download step
- **THEN** the model path used by the generated server configuration is the path that step writes, and the server loads the model

### Requirement: Harness hygiene gates

The verification harness SHALL be covered by tests that run without a live server or ONNX model, and the repository harness SHALL expose checks that report unreachable production code and test coverage.

#### Scenario: Unit tests run without a server

- **WHEN** the harness unit-test target runs
- **THEN** it completes successfully with no running `emb` server, no downloaded model, and no ONNX runtime library

#### Scenario: Dead code is reported

- **WHEN** the dead-code target runs
- **THEN** it lists functions in the module's production code that no production entry point can reach, and exits non-zero when any are found

#### Scenario: Coverage is reported

- **WHEN** the coverage target runs
- **THEN** it reports per-package statement coverage for the verifier and model-loading packages, so an untested reference path is visible
