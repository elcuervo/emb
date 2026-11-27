# Spec Delta

## ADDED Requirements

### Requirement: Image request events report measured phases

For `EMB.IMG` and `EMB.IMGMULTI`, each completed `MONITOR` event SHALL distinguish preprocessing time, image-session dispatch wait, CPU admission wait, ONNX run time, and cache-served items. A phase that cannot be measured SHALL be explicitly unavailable, never reported as measured zero work.

#### Scenario: Image cache miss reports measured inference phases
- **WHEN** an image request misses the reply cache, is preprocessed, waits for an image session or admission, and runs inference
- **THEN** its event reports the measured preprocessing, dispatch, admission, and run durations separately
- **AND** it reports the request's image item count and cache-served count

#### Scenario: Fully cached image request reports no inference work
- **WHEN** all requested image outputs are served from the reply cache
- **THEN** the event identifies the cached items and reports no measured preprocessing, dispatch, admission, or run work
- **AND** it does not present unavailable phases as zero-duration measurements

#### Scenario: Partial cache hit attributes only misses
- **WHEN** one image output is cached and another image output requires inference
- **THEN** the event reports cached and inferred work separately, and phase durations cover only work actually performed

### Requirement: Image phase totals are available per model

The existing model observability surfaces SHALL report cumulative image preprocessing, dispatch, admission, and inference-run durations, completed image runs, and current/total image-session counts without initializing an unloaded image session pool.

#### Scenario: Image model resource totals are visible
- **WHEN** `EMB.INFO` or `EMB.STATS` is read for a model with image traffic
- **THEN** it reports image phase totals and image-session utilization separately from text and script resources

#### Scenario: Reading image metrics does not open sessions
- **WHEN** image metrics are read for a model whose image sessions have not been opened
- **THEN** the metrics report the current unloaded state without initializing the image plan or sessions
