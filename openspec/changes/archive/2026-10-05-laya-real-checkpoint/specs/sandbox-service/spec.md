# Spec Delta

## MODIFIED Requirements

### Requirement: The showcase mounts a shipped miniature decision model by digest

The sandbox SHALL serve a decision-model plate from a shipped miniature (no network
download) AND from a real published checkpoint, alongside its downloaded retrieval
models. The shipped miniature SHALL remain the model behind the plate's game loops,
mounted read-only, its preset preloaded and callable by digest. The plate's
typed-question examples SHALL answer with the real checkpoint, mounted from a
published ONNX export (a one-time download onto the volume), its preset preloaded
and callable by digest. The sandbox's refusal of raw script evaluation SHALL
continue to hold for both.

#### Scenario: The decision plate calls by digest

- **WHEN** a visitor runs the decision demo
- **THEN** the bridge accepts `EMB.EVSHA <model> <digest> …` for each shipped preset, where the digest matches the shipped bytes, and refuses sending script source exactly as it does for the other models

#### Scenario: First boot needs no download for the miniature

- **WHEN** the sandbox boots with the miniature configured
- **THEN** the model and preset files are present in the image, so the loops work without a model download or a volume write

#### Scenario: The real checkpoint downloads once

- **WHEN** the sandbox boots with the real checkpoint configured and the volume does not already hold it
- **THEN** the server downloads it onto the volume, and every later boot loads it from disk without a download

#### Scenario: The typed-question examples answer with the real checkpoint

- **WHEN** a visitor runs the Inbox or Quickstart example
- **THEN** the call is served by the real checkpoint, not the miniature
