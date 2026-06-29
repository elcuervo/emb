## ADDED Requirements

### Requirement: Embedding output matches reference Python implementation

The server SHALL validate that its auto-configured embeddings match Python sentence-transformers output within a cosine similarity threshold.

#### Scenario: Validation generates reference embeddings in Python

- **WHEN** the user runs `just verify-embeddings`
- **THEN** a Python script generates reference embeddings for a test set (20 sentences) using `sentence-transformers` and saves them to a JSON file

#### Scenario: Go embeddings compared against reference

- **WHEN** the validation script runs
- **THEN** the same 20 sentences are embedded via the running emb server using the auto-configured model, and each embedding is compared to its reference using cosine similarity

#### Scenario: Validation passes with high similarity

- **WHEN** the cosine similarity between Go and reference embeddings is computed
- **THEN** all 20 pairs must have cosine similarity > 0.999
- **THEN** the script reports pass/fail per sentence and exits with code 0 on pass

#### Scenario: Model file not found

- **WHEN** the validation model is not downloaded
- **THEN** the script reports a clear error message and exits with code 1
