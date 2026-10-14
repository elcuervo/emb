# emb-stress-load Specification

## Purpose

Synthetic embedding load driven across a DNS-resolved fleet, parameterized by an `emb` server configuration, so an operator can qualify a new cluster and learn which of its resources limits inference.

## Requirements

### Requirement: emb-stress takes a server configuration and a target

`emb-stress` SHALL accept an `emb` server configuration file and a target that is either `host:port` or a DNS name, and SHALL expand a DNS name to one endpoint per address record it resolves to. A target that resolves to no address SHALL be an error naming the target, not a run reported against an empty fleet.

#### Scenario: A name expands to every address record

- **WHEN** `emb-stress` is given a name that resolves to three addresses
- **THEN** load is offered to all three endpoints

#### Scenario: A target that resolves to nothing is an error

- **WHEN** the target resolves to no address
- **THEN** the run reports a clear error naming the target, and does not present an empty fleet as a result

#### Scenario: A literal address runs against one endpoint

- **WHEN** the target is `host:port`
- **THEN** only that endpoint is exercised

### Requirement: Load is offered to every resolved endpoint

`emb-stress` SHALL offer load to every resolved endpoint concurrently, and SHALL report each endpoint's own result separately from the fleet aggregate, so one endpoint's throughput, latency or error rate is attributable to it rather than absorbed by the fleet.

#### Scenario: Every endpoint receives load

- **WHEN** a run drives a fleet of three nodes
- **THEN** the result reports a per-node section for each of the three, and one fleet aggregate

#### Scenario: A slow endpoint does not hide in the aggregate

- **WHEN** one endpoint's latency is far above its peers'
- **THEN** its own reported latency shows it, while the aggregate still includes its samples

### Requirement: Synthetic texts are deterministic and derived from the configuration

`emb-stress` SHALL generate the request texts itself from a seed, with each text's length derived from the configured model's maximum length, so a run needs no corpus file and two runs with the same seed offer the same request set. A corpus file SHALL be accepted as an override.

#### Scenario: The same seed offers the same texts

- **WHEN** two runs use the same seed and configuration
- **THEN** they offer the same sequence of texts

#### Scenario: Lengths follow the configured maximum

- **WHEN** a model is configured with a maximum length
- **THEN** the generated texts vary across length classes bounded by that maximum, and no offered text exceeds it

#### Scenario: A corpus overrides generation

- **WHEN** a corpus file is supplied
- **THEN** requests are drawn from it instead of the generator

### Requirement: The configured models are reconciled against the live fleet

`emb-stress` SHALL compare the models named in the configuration with those each endpoint reports, and SHALL name any endpoint that is missing a configured model or reports a different embedding dimension, rather than exercising it silently under a name it does not serve.

#### Scenario: A missing model is named

- **WHEN** the configuration declares a model that an endpoint does not report
- **THEN** the result names that endpoint and the missing model

#### Scenario: A dimension mismatch is named

- **WHEN** an endpoint reports a different dimension for a configured model
- **THEN** the result names that endpoint and both dimensions

#### Scenario: A matching fleet runs clean

- **WHEN** every endpoint reports every configured model at the expected dimension
- **THEN** the reconciliation reports no drift

### Requirement: Each load level is held before it is measured

`emb-stress` SHALL hold each load level for a configured dwell and SHALL discard a configured warm-up portion of it, so a level's reported numbers describe a fleet that has settled — caches filled, queue at its steady depth — rather than one still filling them.

#### Scenario: Warm-up samples are not reported

- **WHEN** a level runs with a warm-up
- **THEN** the level's reported throughput, latency and error rate are drawn only from the samples after the warm-up

#### Scenario: A level reports only its own dwell

- **WHEN** a level's dwell ends
- **THEN** its result covers the settled portion of that dwell and no earlier level's samples

### Requirement: The run reports per-endpoint and fleet results

`emb-stress` SHALL report, per endpoint and for the fleet, the offered load, completed requests per second, latency percentiles and the error count, in a machine-readable form as well as a human one. The reported latency SHALL be the client's own measurement, and no server-reported value SHALL be substituted for it.

#### Scenario: Results are machine-readable

- **WHEN** a run completes
- **THEN** it emits a machine-readable result containing every endpoint's numbers and the fleet aggregate

#### Scenario: Latency is the client's own measurement

- **WHEN** the result reports a latency
- **THEN** it is derived from the client's own send-to-reply timing, not from a server counter

#### Scenario: Errors are counted, not hidden

- **WHEN** some requests fail
- **THEN** the result reports the error count beside the success rates, and does not count a failed request as a completed one
