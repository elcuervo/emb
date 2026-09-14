# script-eval delta

## MODIFIED Requirements

### Requirement: Sandboxed execution with budgets

Script execution SHALL be sandboxed and isolated: each evaluation runs in a fresh interpreter with only whitelisted host functions and a curated standard-library subset; `io`, `os`, `module`, file/network access, FFI, `math.random`, and all time functions SHALL be unavailable. The whitelisted surface SHALL be exactly: `emb.run`, `emb.run_batch`, `emb.embed` (models with an embedding configuration), `emb.tokenize.pretokenized`, `emb.tokenize.words`, `emb.tokenize.encode`, `emb.tokenize.encode_pair`, `emb.math`, `emb.similarity`, `emb.distance`, `emb.image.preprocess` / `emb.image.info` / `emb.image.embed` (models with an image configuration), `emb.API_VERSION`, and `json`. Each evaluation SHALL be bounded by a wall-clock deadline (enforced at VM instruction granularity), a call-stack depth limit, a script-size cap, and a tensor-element budget; exceeding any bound replies with an error for that request only and never affects other in-flight requests. Scripted evaluation commands SHALL count toward the server's `max_concurrent_requests` gate.

Every host function SHALL be pure compute over the model and request inputs: no host function may perform network access, read the clock or randomness, or expose server state, so that identical inputs produce identical replies and the content-addressed reply cache stays correct.

#### Scenario: Infinite loop in one request

- **WHEN** a script runs past its wall-clock deadline
- **THEN** that request replies with an execution-time error while concurrent requests continue normally

#### Scenario: Oversized script

- **WHEN** a script exceeds the configured size cap
- **THEN** the server replies with an error and does not compile or cache it

#### Scenario: Forbidden library is absent

- **WHEN** a script attempts to use `os` or `io` functions
- **THEN** the script fails with an unknown-function error at runtime (or compile time), never touching the host

#### Scenario: New host functions are pure compute

- **WHEN** a script calls `emb.embed`, `emb.similarity`, `emb.distance`, or `emb.image.embed`
- **THEN** the reply depends only on the model, the request inputs, and the loaded weights, and repeating the evaluation produces identical replies

## ADDED Requirements

### Requirement: Script API version

The server SHALL expose `emb.API_VERSION` as a string describing the host function surface available to scripts. The value SHALL change when host functions are added, removed, or change semantics, and SHALL remain stable for changes that only affect performance. Scripts SHALL be able to compare it (for example by major version prefix) and report a capability error themselves; the server SHALL NOT refuse to evaluate a script on version grounds.

#### Scenario: Version is readable

- **WHEN** a script returns `emb.API_VERSION`
- **THEN** the reply is a non-empty string

#### Scenario: Version reflects the extended surface

- **WHEN** a script asserts that `emb.similarity` and packed-output `emb.run` are available and the server version predates them
- **THEN** the script can detect the absence and reply with its own error instead of failing at the call site

#### Scenario: Version is stable across performance work

- **WHEN** only performance characteristics of existing host functions change
- **THEN** `emb.API_VERSION` is unchanged
