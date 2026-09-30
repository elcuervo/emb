## ADDED Requirements

### Requirement: The docs document the Laya decision-model integration

The docs surface SHALL document the Laya decision-model path: mounting an export bundle as a scripted model, preloading the `laya.lua` preset, and the `EMB.EVSHA` wire contract — state as `KEYS` text (Python-style serialization for structured states), questions as a JSON object whose criteria label order is significant, the optional config envelope (`max_len`, `head_max_len`, `min_seq`, `min_markers`, `temperature`, `temperature_by_options`), the question schema (`choice`/`score`/`noul`), and the calibration clamps (temperatures outside [0.5, 5.0] clamp; non-numeric values answer with 1.0). It SHALL state the parity basis (the vendored corpus pins the preset byte-for-byte) and SHALL note that the website sandbox serves a miniature stand-in whose numbers are not judgments.

#### Scenario: A reader can reproduce the command

- **WHEN** a reader follows the docs section with a local `emb` and an export bundle
- **THEN** the documented command and config envelope run the same preset and produce the documented payload shapes

#### Scenario: The sandbox stand-in is called out

- **WHEN** the docs mention the sandbox decision demo
- **THEN** they state that the sandbox runs the 32-hidden miniature for mechanism only, and point to the published checkpoints for real answers