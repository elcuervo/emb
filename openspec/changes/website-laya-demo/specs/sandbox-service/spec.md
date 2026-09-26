## ADDED Requirements

### Requirement: The showcase mounts a shipped miniature decision model by digest

The sandbox SHALL serve a decision-model plate from a model and preset shipped inside the image (no network download), alongside its downloaded retrieval models. The model SHALL be the vendored tiny Laya export, mounted read-only, and its preset SHALL be the shipped `laya.lua` preloaded and callable by digest like every other preset; the sandbox's refusal of raw script evaluation SHALL continue to hold for it.

#### Scenario: The decision plate calls by digest

- **WHEN** a visitor runs the decision demo
- **THEN** the bridge accepts `EMB.EVSHA laya <digest> …` where the digest matches the shipped preset, and refuses sending script source exactly as it does for the other models

#### Scenario: First boot needs no download for the miniature

- **WHEN** the sandbox boots with the decision model configured
- **THEN** the model and preset files are present in the image, so the plate works without a model download or a volume write