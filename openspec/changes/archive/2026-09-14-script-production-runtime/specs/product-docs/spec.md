# product-docs delta

## ADDED Requirements

### Requirement: Script API reference and production guide

The documentation surface SHALL carry a script API reference covering every host function available to scripts, and a production scripting guide. The reference SHALL present functions grouped by module (`emb.embed`, `emb.run`/`emb.run_batch`, `emb.tokenize`, `emb.math`, `emb.similarity`/`emb.distance`, `emb.image`), and for each function SHALL state its signature, accepted operand forms (per-element array vs packed `bytes`), return shape, error conditions, and whether it is available only for models with an embedding or image configuration.

The production guide SHALL state, at minimum:

- the determinism and reply-cache contract, including that a script's reply must depend only on (model, script SHA1, args, text), and the one-KEY-plus-ARGV idiom for pairwise scoring;
- how to obtain embeddings (`emb.embed` / `emb.image.embed`) versus raw graph tensors (`emb.run`), and when each is appropriate;
- the packed-buffer workflow for large outputs, and that per-element materialization is the dominant cost when avoided;
- the configured limits (deadline, script size, tensor elements) and how they fail;
- the memory and observability characteristics of scripted models (session count, `script_workers`, how to see them in `EMB.STATS`/`MONITOR`).

Example scripts SHALL be classified as **reference** (maintained, exercised by CI against a testbed model) or **snippet** (illustrative, not maintained), and the docs SHALL say which is which.

#### Scenario: Every host function is documented

- **WHEN** the script API reference is read
- **THEN** each function exposed under `emb.*` by the sandbox is described with its signature, operand forms, and errors

#### Scenario: Similarity and distance semantics are explicit

- **WHEN** the reference documents `emb.similarity` and `emb.distance`
- **THEN** it states that `similarity` returns higher-is-more-similar and `distance` returns lower-is-closer, and lists each supported metric with its formula

#### Scenario: Maintained scripts are distinguishable

- **WHEN** a reader opens the examples directory through the docs
- **THEN** the maintained reference scripts and the illustrative snippets are separated, and the GLiNER2 extractor is listed as maintained

#### Scenario: Production constraints are stated

- **WHEN** the production guide is read
- **THEN** it states the determinism/caching contract, the limits and their failure modes, and how scripted traffic appears in `EMB.STATS` and `MONITOR`
