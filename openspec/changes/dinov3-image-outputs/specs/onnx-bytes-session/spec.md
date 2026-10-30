## MODIFIED Requirements

### Requirement: ONNX sessions load from pre-read bytes

The system SHALL provide `NewRuntimeSessionFromBytes(data []byte, modelPath string, inputNames, outputNames []string, dim, outputRank, intraOpThreads, interOpThreads, execMode int, allowSpinning bool)` in `internal/onnx` that creates an ORT session from an in-memory byte slice using `ort.NewDynamicAdvancedSessionWithONNXData`, built with the same session options (`newSessionOptions(intraOpThreads, interOpThreads, execMode)`) as every other runtime session. The earlier path-based `NewRuntimeSession` constructor is removed; nothing in the module reached it, so the bytes constructor is the only way sessions are created.

A graph whose weights live outside the graph file SHALL load from those pre-read bytes: the session SHALL be told the directory the graph was read from through `session.model_external_initializers_file_folder_path`, so ORT resolves its external initializers (for example a `<graph>.onnx.data` sidecar written beside it) from that directory rather than from the process working directory. The same directory SHALL be supplied by the named-session constructor used for image and scripted sessions. A missing or unreadable initializer file SHALL fail session creation with an error naming the model path and the directory that was searched. A graph that holds its weights inline SHALL be unaffected.

#### Scenario: Session created from bytes succeeds

- **WHEN** valid ONNX model bytes are passed to `NewRuntimeSessionFromBytes`
- **THEN** a `*RuntimeSession` is returned with no error and inference runs correctly

#### Scenario: Invalid bytes return error

- **WHEN** malformed or empty bytes are passed to `NewRuntimeSessionFromBytes`
- **THEN** an error is returned and no session is created

#### Scenario: External initializers resolve beside the graph

- **WHEN** a model file carries external initializers in a sidecar file next to the graph, and sessions are built from the graph's bytes
- **THEN** the session SHALL load and run without depending on the process working directory

#### Scenario: Missing sidecar is a named failure

- **WHEN** the graph references external initializers that are absent from its directory
- **THEN** session creation SHALL fail with an error naming the model path and the searched directory
