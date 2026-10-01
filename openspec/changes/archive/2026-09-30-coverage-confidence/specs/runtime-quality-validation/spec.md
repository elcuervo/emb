# Spec Delta

## ADDED Requirements

### Requirement: Model auto-configuration regression tests

The model-loading boundary SHALL have focused tests proving that a model configured without explicit dimension, maximum length, output tensor, or pooling mode is configured from its graph and `config.json`, with a defined fallback when detection fails.

#### Scenario: Detection is exercised

- **WHEN** a model is loaded with `Dim`, `MaxLength`, `OutputTensor`, and `Pooling` left unset
- **THEN** a test SHALL cover detection of the dimension, maximum length, output tensor, and pooling mode from the model's metadata

#### Scenario: Detection fallback is exercised

- **WHEN** the model's metadata cannot supply a value (missing or unparseable `config.json`, no rank-2/rank-3 output)
- **THEN** a test SHALL cover the documented fallback value rather than a silent zero

### Requirement: Cached script reply encoding regression tests

The bytes stored for and replayed from the script reply cache SHALL be tested for every reply shape the server can cache, including null and error replies, not only the shapes the live-connection writer handles.

#### Scenario: Null and error encodings are covered

- **WHEN** a script result is a null value or an error table
- **THEN** a test SHALL assert the encoded cache bytes for that result
- **AND** the encoded bytes SHALL match what the live connection writes for the same result

### Requirement: Image resource construction regression tests

Image session pool construction SHALL have focused tests that do not require a real vision export: successful open, closure, and rollback when a later session fails to construct.

#### Scenario: Image pool lifecycle is covered

- **WHEN** an image session pool is constructed
- **THEN** a test SHALL cover successful construction, that every opened session is closed exactly once, and that sessions opened before an injected construction failure are rolled back
