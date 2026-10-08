# Spec Delta

## MODIFIED Requirements

### Requirement: Every demo teaches the same five things in the same order

Each demo SHALL present, in one fixed order: what the reader is looking at, the
interaction itself, what just happened (the path from the input — text, or a
picture of a sound or a clip — through a model to a vector to a distance), why
it matters (the real-world job the demo stands in for), and the exact commands
the demo issued. A demo MUST NOT offer an interaction without the mechanism that
produced it and the commands that ran it, and the section headings MUST be the
same across demos so the gallery reads as one curriculum rather than a set of
unrelated toys.

#### Scenario: A demo carries all five sections

- **WHEN** any demo page is rendered
- **THEN** it presents what it is, the interaction, the mechanism, the real-world use, and the commands, in that order

#### Scenario: The mechanism is named, not implied

- **WHEN** the mechanism section is read
- **THEN** it states that the input is encoded by a named model, pooled and normalized into a fixed-length vector, and compared by a distance, rather than describing the result as magic

#### Scenario: The commands are the ones that ran

- **WHEN** a demo issues a command to the sandbox
- **THEN** the command shown to the reader is the command that was issued, including the model name, the reply form, and the arguments the demo actually sent

### Requirement: The corpus is fixed and the visitor's text is ephemeral

Every demo SHALL search a corpus of items committed to the repository and
embedded before deployment. Text a visitor submits MUST be used only to answer
that request and MUST NOT be stored, added to a shared corpus, or made visible
to another visitor. A demo MUST NOT introduce a server-side write, a visitor
account, or mutable shared state, and the sandbox's read-only posture MUST be
preserved.

#### Scenario: Submitted text leaves no trace

- **WHEN** a visitor submits text to a demo
- **THEN** the text is used for the request and is not persisted, indexed into the gallery's corpus, or returned to any other visitor

#### Scenario: The corpus cannot change at runtime

- **WHEN** the gallery is running
- **THEN** no visitor action adds, removes, or modifies an item in a demo's corpus

#### Scenario: No demo requires an account

- **WHEN** a demo is used
- **THEN** it asks for no credential, no identity, and no stored preference

### Requirement: The search is a real vector search over the committed index

Each retrieval demo SHALL run an exact top-k search against a vector index over
the committed corpus — its texts, its images, or its media — built from vectors
the sandbox's own model produced. A demo MUST NOT present a precomputed or
hard-coded ranking as a live result. The query MUST be embedded by the same
model that embedded the corpus, and a disagreement MUST fail legibly rather than
return meaningless neighbours.

#### Scenario: The ranking is computed, not stored

- **WHEN** a visitor runs a query
- **THEN** the ranking is produced by searching the index with the query's vector, and a query the corpus was not built to anticipate still returns the nearest items

#### Scenario: Both sides use one model

- **WHEN** a corpus is embedded and later queried
- **THEN** the same model and the same normalization produce both sides, and the demo reports which model

#### Scenario: A model mismatch fails loudly

- **WHEN** a demo's index and its query model disagree, or the index is missing
- **THEN** the demo states the failure instead of rendering a ranking
