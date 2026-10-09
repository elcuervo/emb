# Spec Delta

## ADDED Requirements

### Requirement: One committed index backs every demo

The gallery SHALL ship one content-hashed index artefact and one manifest that
together hold every retrieval the demos perform — the text corpus's vectors, the
media vectors, and the fingerprint library — and a demo MUST read its data, its
models, its dimensions, and its counts from that artefact rather than from a
per-demo file.

#### Scenario: Every retrieval lives in one file

- **WHEN** the gallery's index is built
- **THEN** the text vectors, the media vectors, and the fingerprint library are tables in the same content-hashed file, described by one manifest, and no per-demo index file ships

#### Scenario: A demo reads its own numbers from the shared manifest

- **WHEN** a plate displays a count, a dimension, a model name, or a licence
- **THEN** the value comes from the shared manifest or the shared index, so a rebuild changes the page without a page edit

#### Scenario: A vector table declares its element type

- **WHEN** the manifest describes a vector table
- **THEN** it states that table's element type and dimension, and a table whose recall was measured is labelled with its precision

### Requirement: The gallery searches through a single implementation

Every plate that ranks vectors SHALL search through the gallery's one
client-side vector-search implementation, over the one committed index. A plate
MUST NOT carry its own vector decoding or cosine scoring.

#### Scenario: A plate ranks through the shared path

- **WHEN** a plate needs ranked neighbours for a query vector
- **THEN** it calls the shared search path with its model, its vector, and its `k`, and the shared path returns the ranking

#### Scenario: No plate ships its own similarity code

- **WHEN** a plate is reviewed
- **THEN** it holds no vector decoder and no cosine loop of its own, and a query vector it obtained from a scripted model call is passed to the shared search path unchanged
