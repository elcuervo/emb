## MODIFIED Requirements

### Requirement: Config format specification

The server SHALL accept optional `preload` and `workers` fields in model config:

```yaml
models:
  <name>:
    onnx: /path/to/model.onnx
    preload: false    # optional, default: false (lazy load)
    workers: 0        # optional, default: 0 (auto-tune by RAM)
```

#### Scenario: Config with preload and workers parsed correctly

- **WHEN** server starts with config containing `preload: true` and `workers: 4`
- **THEN** the model is loaded at startup with exactly 4 workers

#### Scenario: Config without preload defaults to lazy

- **WHEN** server starts with config that omits `preload`
- **THEN** the model is registered but not loaded until the first EMB request
