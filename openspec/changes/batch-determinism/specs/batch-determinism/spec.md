# batch-determinism Specification

## Purpose
Specifies that embeddings served by a batched model are a deterministic function of the input text and the model, independent of batch composition, enforced by a load-time probe that runs by default with no configuration.

## ADDED Requirements

### Requirement: Batched embeddings deterministic per text

The server SHALL produce byte-identical embeddings for a given text and model regardless of which other texts share its inference batch, for every model that batches under load.

#### Scenario: Batch composition invariance

- **WHEN** a model serves a text in a 1-row run and in a multi-row batch run on the same process and model build
- **THEN** the returned embedding bytes SHALL be identical

#### Scenario: Deterministic graphs keep batching

- **WHEN** a model's graph is batch-invariant (e.g. fp32, or int8 with static activation scales)
- **THEN** batching SHALL remain enabled and the server SHALL report `batch_determinism: passed`

### Requirement: Load-time determinism probe

The server SHALL probe batch-determinism at model load whenever batching is enabled, caching the verdict for the model's lifetime.

#### Scenario: Probe runs at load

- **WHEN** a model loads with `batching.timeout > 0`
- **THEN** the server SHALL embed fixed probe texts alone and co-batched and byte-compare the outputs
- **THEN** the verdict SHALL be recorded once and reused for the model's lifetime

#### Scenario: Probe skipped when batching is off

- **WHEN** a model loads with `batching.timeout: 0` (worker pool, never batched)
- **THEN** no probe SHALL run and `batch_determinism` SHALL report `untested`

### Requirement: Probe gates batching

The server SHALL disable batching for any model whose probe fails, with no policy to re-enable it.

#### Scenario: Probe failure degrades to unbatched

- **WHEN** a model's probe fails
- **THEN** the model SHALL serve unbatched (worker pool, single-row inference)
- **THEN** the server SHALL log the degradation and its reason on a greppable line

#### Scenario: Probe pass keeps batching

- **WHEN** a model's probe passes
- **THEN** batching SHALL remain enabled and the server SHALL report `batch_determinism: passed`

### Requirement: Determinism observability

The server SHALL report each model's determinism verdict and effective batching state.

#### Scenario: EMB.INFO and EMB.STATS surface the verdict

- **WHEN** `EMB.INFO <model>` or `EMB.STATS` is called for a model
- **THEN** the reply SHALL include `batch_determinism` (`passed`, `failed`, or `untested`) and the effective `batching_timeout` after gating

#### Scenario: Degradation is visible

- **WHEN** a model degrades to unbatched after a failed probe
- **THEN** the effective batching timeout SHALL read `0` and the reason SHALL be present in `EMB.INFO`