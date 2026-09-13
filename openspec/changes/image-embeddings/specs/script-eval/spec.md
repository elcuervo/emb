## MODIFIED Requirements

### Requirement: Sandboxed execution with budgets

Script execution SHALL be sandboxed and isolated: each evaluation runs in a fresh interpreter with only whitelisted host functions (`emb.run`, `emb.run_batch`, `emb.tokenize.pretokenized`, `emb.tokenize.words`, `emb.tokenize.encode`, `emb.tokenize.encode_pair`, `emb.math`, `emb.image.preprocess`, `emb.image.info`, `json`) and a curated standard-library subset; `io`, `os`, `module`, file/network access, FFI, `math.random`, and all time functions SHALL be unavailable. Text arguments (KEYS) SHALL be binary-safe: arbitrary bytes SHALL be delivered to the script unmodified, so image content can be passed as a KEYS element without encoding. Each evaluation SHALL be bounded by a wall-clock deadline (enforced at VM instruction granularity), a call-stack depth limit, and a script-size cap; exceeding any bound replies with an error for that request only and never affects other in-flight requests. Scripted evaluation commands SHALL count toward the server's `max_concurrent_requests` gate.

#### Scenario: Infinite loop in one request

- **WHEN** a script runs past its wall-clock deadline
- **THEN** that request replies with an execution-time error while concurrent requests continue normally

#### Scenario: Oversized script

- **WHEN** a script exceeds the configured size cap
- **THEN** the server replies with an error and does not compile or cache it

#### Scenario: Forbidden library is absent

- **WHEN** a script attempts to use `os` or `io` functions
- **THEN** the script fails with an unknown-function error at runtime (or compile time), never touching the host

#### Scenario: Binary KEYS are delivered unmodified

- **WHEN** an `EMB.EVAL`/`EMB.EVSHA` text argument contains arbitrary bytes (for example image content with NUL and high-bit bytes)
- **THEN** the script receives those exact bytes in `KEYS`, and the sandbox remains network-free

## ADDED Requirements

### Requirement: Image preprocessing host block

For a model configured with an `image:` block, the server SHALL provide `emb.image.preprocess(bytes)` to scripts: it SHALL decode the raw image bytes and apply the model's configured image preprocessing (size, crop, resample, rescale, mean, std), returning a tensor spec `{shape = {1, 3, H, W}, bytes = <little-endian float32>, dtype = "f32", input = <configured input tensor name>}` that can be passed directly to `emb.run` or `emb.run_batch`. The server SHALL also provide `emb.image.info()` returning the model's configured preprocessing parameters. Preprocessing SHALL be deterministic (identical bytes and config produce identical output), SHALL be bounded by the same per-image byte and decoded-pixel caps as `EMB.IMG`, and SHALL be charged against the evaluation's tensor budget. Calling `emb.image.preprocess` for a model without an `image:` block, or on undecodable bytes, SHALL raise an error. This block SHALL add no network capability.

#### Scenario: Image bytes become a runnable tensor

- **WHEN** a script calls `emb.image.preprocess(KEYS[1])` with image bytes for a model with `image: {input: pixel_values, size: 224}`
- **THEN** it receives `{shape = {1, 3, 224, 224}, bytes = <4*3*224*224 bytes>, dtype = "f32", input = "pixel_values"}` that `emb.run` accepts without a per-element Lua table

#### Scenario: Deterministic and cacheable

- **WHEN** the same script runs twice with identical binary KEYS and args
- **THEN** the preprocessed tensor is byte-identical and the reply is served from cache on the second request

#### Scenario: No image config errors

- **WHEN** a script calls `emb.image.preprocess` for a model without an `image:` block
- **THEN** the evaluation fails with an error naming the model

#### Scenario: Image block adds no network

- **WHEN** a script uses `emb.image.preprocess`
- **THEN** the sandbox remains network-free and no fetch is performed

### Requirement: Bounded script reply-cache keys

The content-addressed script reply-cache key SHALL NOT inline large text payloads. When a KEYS element exceeds a small documented threshold, the key SHALL incorporate a digest of that element instead of its raw bytes, so image-sized KEYS do not retain megabytes per cache entry. Hit/miss semantics SHALL be unchanged: identical KEYS and args always map to the same key, and distinct KEYS always map to distinct keys. Short text elements SHALL continue to produce the same keys as before, so existing cached text entries remain reachable.

#### Scenario: Image KEYS do not bloat the key

- **WHEN** a scripted image request is cached
- **THEN** the cache key size is bounded (a digest, not the full image bytes) and the reply is identical to an uncached run

#### Scenario: Text keys are unchanged

- **WHEN** a short text scripted request is cached
- **THEN** its key matches the pre-change key format and previously cached entries still hit

#### Scenario: Distinct images remain distinct entries

- **WHEN** two different images are embedded through the same script
- **THEN** they occupy distinct cache entries and each returns its own embedding
