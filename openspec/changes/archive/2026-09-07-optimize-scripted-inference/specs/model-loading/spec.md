## ADDED Requirements

### Requirement: Scripted inference configuration

`ModelConfig` SHALL support `script_workers` and `script_preload` keys alongside the embedding-pool keys. `script_workers` bounds parallel scripted sessions for the model: unset/0 auto-tunes by RAM and model size, explicit N creates exactly N (minimum 1). `script_preload: true` SHALL warm the model's scripted session and tokenizer at load time instead of on first script evaluation.

#### Scenario: Explicit script workers parsed and applied

- **WHEN** a model declares `script_workers: 2`
- **THEN** exactly 2 scripted sessions exist for the model once scripted evaluation begins

#### Scenario: Script preload at startup

- **WHEN** a model declares `script_preload: true`
- **THEN** the scripted session and tokenizer are created during model loading, and the first `EMB.EVSHA` request does not pay session-creation latency