# onnx-bytes-session Specification

## Purpose
TBD - created by archiving change load-onnx-model-from-bytes. Update Purpose after archive.
## Requirements
### Requirement: ONNX sessions load from pre-read bytes
The system SHALL provide `NewRuntimeSessionFromBytes(data []byte, inputNames, outputNames []string, dim, outputRank, intraOpThreads, interOpThreads int)` in `internal/onnx` that creates an ORT session from an in-memory byte slice using `ort.NewDynamicAdvancedSessionWithONNXData`, with identical session options to `NewRuntimeSession`.

#### Scenario: Session created from bytes succeeds
- **WHEN** valid ONNX model bytes are passed to `NewRuntimeSessionFromBytes`
- **THEN** a `*RuntimeSession` is returned with no error and inference runs correctly

#### Scenario: Invalid bytes return error
- **WHEN** malformed or empty bytes are passed to `NewRuntimeSessionFromBytes`
- **THEN** an error is returned and no session is created

### Requirement: Registry reads model file once per pool initialization
The system SHALL read the model file into memory once with `os.ReadFile(cfg.ONNX)` inside `ensurePool` before constructing the session factory, and each worker session SHALL be created via `NewRuntimeSessionFromBytes` using that shared byte slice.

#### Scenario: N workers created from one file read
- **WHEN** a model pool is initialized with `numWorkers = N`
- **THEN** the model file is read from disk exactly once and N sessions are created from the in-memory bytes

#### Scenario: NFS file not accessed during inference
- **WHEN** inference is called after pool initialization
- **THEN** no filesystem reads occur on the model file path during the inference call

