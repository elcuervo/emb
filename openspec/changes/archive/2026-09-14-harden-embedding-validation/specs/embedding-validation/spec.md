## MODIFIED Requirements

### Requirement: Embedding output matches reference Python implementation

The server SHALL validate that its auto-configured embeddings match Python sentence-transformers output within a documented cosine similarity threshold, using a reference artifact that is pinned, carries provenance, and is validated before it is trusted. The verification SHALL run against a running `emb` server and SHALL report a clear, non-zero failure when the model, the reference artifact, or the server is unavailable.

#### Scenario: Validation generates reference embeddings in Python

- **WHEN** the user runs the reference-embedding verification and no reference artifact exists
- **THEN** a Python script generates reference embeddings for the test set using `sentence-transformers`, with the model and the dependency versions pinned to recorded values
- **AND** the artifact records the model name, dimension, sentence set, generator version, and a checksum over the embeddings

#### Scenario: Reference artifact is validated before use

- **WHEN** a reference artifact exists but its recorded model, dimension, sentence set, or checksum does not match the artifact's contents or the verification inputs
- **THEN** the verification SHALL refuse to compare against it and SHALL either regenerate it or exit non-zero naming the mismatch

#### Scenario: Go embeddings compared against reference

- **WHEN** the verification runs against a reachable server
- **THEN** the same sentences are embedded via the running emb server using the configured model, and each embedding is compared to its reference using cosine similarity

#### Scenario: Validation passes with high similarity

- **WHEN** the cosine similarity between each served and reference embedding is computed
- **THEN** every pair must meet the cosine threshold documented for this check, and the threshold SHALL be reported and configurable rather than compiled in
- **THEN** the script reports pass/fail per sentence and exits with code 0 only when every pair meets the threshold

#### Scenario: Model file not found

- **WHEN** the validation model cannot be loaded by the server
- **THEN** the script reports a clear error message naming the model and exits with code 1

#### Scenario: Reference artifact missing

- **WHEN** the verification runs and no reference artifact can be found or generated
- **THEN** it SHALL report a clear error naming the expected artifact and exit with a non-zero code

#### Scenario: Server unavailable or reply is not an embedding

- **WHEN** the verification cannot connect to the server, or the server replies with an error instead of an embedding
- **THEN** it SHALL report the connection or server error, name the sentence that failed, and exit with a non-zero code rather than reporting a similarity
