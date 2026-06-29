## ADDED Requirements

### Requirement: Lazy model loading

Models SHALL be loaded on first `EMB <model>` request by default, not at server startup.

#### Scenario: Lazy load on first request

- **WHEN** server starts with `preload: false` (default) for a model
- **THEN** the model's ONNX session and worker pool are not created at startup
- **THEN** on the first `EMB <model>` command, the model is loaded before processing the request

#### Scenario: Preloaded model at startup

- **WHEN** a model has `preload: true` in config
- **THEN** the model's ONNX session and worker pool are created at startup (current behavior)

#### Scenario: First request blocks until loaded

- **WHEN** the first `EMB <model>` request triggers a lazy load
- **THEN** the request blocks until the model is fully loaded and then processes normally
- **THEN** subsequent requests use the already-loaded pool

### Requirement: Auto-tuned worker count

The server SHALL auto-tune worker pool size based on available system RAM and model file size.

#### Scenario: Auto-tune by default

- **WHEN** a model has `workers: 0` (default) in config
- **THEN** the worker count is computed as `min(GOMAXPROCS, available_memory * 0.5 / (model_file_size * 1.2))` with a minimum of 1

#### Scenario: Explicit workers override

- **WHEN** a model has `workers: N` in config with N > 0
- **THEN** exactly N workers are created regardless of auto-tune calculation

#### Scenario: Auto-tune prevents OOM

- **WHEN** available RAM is insufficient for the default GOMAXPROCS workers
- **THEN** worker count is reduced to fit within half of available RAM
