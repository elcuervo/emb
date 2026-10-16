# Spec Delta

## MODIFIED Requirements

### Requirement: Lazy model loading

Models SHALL be laid out on first use by default, off the request path, not at server startup.

#### Scenario: Lazy load on first request

- **WHEN** server starts with `preload: false` (default) for a model
- **THEN** the model's ONNX session and worker pool are not created at startup
- **WHEN** the first `EMB <model>` command reaches a model whose layout is undecided
- **THEN** the command SHALL be answered with a not-ready error naming the model, and SHALL NOT wait for the load
- **AND** `EMB.MULTI` SHALL answer that pair with its per-pair null element rather than refusing the whole command
- **AND** the load SHALL be started by that request, single-flight, so concurrent refusals start no second load

#### Scenario: A request after the layout is decided is served

- **WHEN** `EMB.MODELS` reports a lazy model as `ready`
- **THEN** `EMB <model>` SHALL be served normally, and subsequent requests SHALL use the already-loaded pool
- **AND** no request SHALL have blocked on the load for it to reach that state

#### Scenario: Status reports which side of the load a model is on

- **WHEN** a lazy model is unasked-for, loading, or laid out
- **THEN** `EMB.MODELS` SHALL report `uninitialized`, `loading`, or `ready` respectively, and `EMB.READY` SHALL report the process state

#### Scenario: First request blocks until loaded

- **WHEN** the first `EMB <model>` request for a lazy model arrives and its layout is undecided
- **THEN** the request SHALL NOT block until the model is loaded
- **AND** the load it triggered SHALL still be completed in the background, so that later requests are served
- **AND** the refusal, not a blocking wait, is the observable behavior this scenario pins — it keeps its historical name for traceability with the behavior it replaces

#### Scenario: Preloaded model at startup

- **WHEN** a model has `preload: true` in config
- **THEN** the model's ONNX session and worker pool are created at startup (current behavior)
