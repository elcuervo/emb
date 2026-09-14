## MODIFIED Requirements

### Requirement: ONNX sessions load from pre-read bytes

The system SHALL provide `NewRuntimeSessionFromBytes(data []byte, inputNames, outputNames []string, dim, outputRank, intraOpThreads, interOpThreads int)` in `internal/onnx` that creates an ORT session from an in-memory byte slice using `ort.NewDynamicAdvancedSessionWithONNXData`, built with the same session options (`newSessionOptions`) as every other runtime session. The earlier path-based `NewRuntimeSession` constructor is removed; nothing in the module reached it, so the bytes constructor is the only way sessions are created.

#### Scenario: Session created from bytes succeeds

- **WHEN** valid ONNX model bytes are passed to `NewRuntimeSessionFromBytes`
- **THEN** a `*RuntimeSession` is returned with no error and inference runs correctly

#### Scenario: Invalid bytes return error

- **WHEN** malformed or empty bytes are passed to `NewRuntimeSessionFromBytes`
- **THEN** an error is returned and no session is created
