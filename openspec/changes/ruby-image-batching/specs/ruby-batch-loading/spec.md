# Spec Delta

## ADDED Requirements

### Requirement: Image calls honor the configured lazy mode

Image calls SHALL remain eager when `lazy: false` and SHALL defer within the current batch scope under `lazy: :multi` or `lazy: :batch`. Creating a deferred image value SHALL perform no network I/O; forcing a value SHALL resolve its image result.

#### Scenario: Eager image API remains unchanged
- **WHEN** an image is embedded with the default `lazy: false`
- **THEN** `EMB.IMG` is sent during the call and the decoded result is returned

#### Scenario: Deferred image call performs no I/O until forced
- **WHEN** `client[:model].image(bytes)` is called under a deferred mode
- **THEN** no command is sent until the returned value is used
- **AND** forcing the value returns the same decoded result as eager mode

### Requirement: Deferred image calls coalesce within a scope

Deferred image items SHALL be grouped by client, model, format, and output projection. Each group SHALL be chunked by image count and sent as `EMB.IMG`; `:multi` SHALL execute chunks serially, while `:batch` SHALL dispatch independent compatible groups concurrently. Results SHALL be reassembled in original deferral order.

#### Scenario: Same-model images coalesce in multi mode
- **WHEN** several compatible image calls for one model are created under `lazy: :multi` and one is forced
- **THEN** the images are sent in bounded `EMB.IMG` chunks, serially
- **AND** each call receives its result in creation order

#### Scenario: Independent model groups run concurrently in batch mode
- **WHEN** compatible image calls for multiple models are created under `lazy: :batch` and one is forced
- **THEN** each model's bounded `EMB.IMG` chunks are dispatched concurrently
- **AND** each result is reassembled in deferral order

#### Scenario: Different output projections are not merged
- **WHEN** deferred calls for one model request different formats or output projections
- **THEN** they are sent in separate compatible groups and each result uses its requested decoding

### Requirement: Deferred image payload retention is bounded

The client SHALL bound the number of image payloads retained in one deferred scope using the existing `batch_size` limit and the total encoded bytes using `max_image_batch_bytes`, configurable globally and per client with a 32 MiB default. It SHALL reject a deferral that would exceed either bound before adding that item to the scope, with an error identifying the exceeded limit; it SHALL NOT silently drop or truncate an image.

#### Scenario: Image count bound is enforced
- **WHEN** deferring another image would exceed the configured batch image-count limit
- **THEN** the client raises a limit error and does not add or send that image

#### Scenario: Encoded byte bound is enforced
- **WHEN** deferring an image would exceed the configured total encoded-byte limit for the scope
- **THEN** the client raises a limit error and does not add or send that image

### Requirement: Deferred image items follow batch-scope failure and cleanup rules

Deferred image results SHALL preserve the server's per-image null behavior and SHALL be cleared with the existing request/job batch scope on success or failure, without carrying image bytes or results into a later scope.

#### Scenario: Failed or truncated image slot stays nil
- **WHEN** a batched `EMB.IMG` reply contains a null slot
- **THEN** only the corresponding image result is nil and other results are decoded normally

#### Scenario: Image payloads are released at scope end
- **WHEN** a request or job batch scope is cleared, including after an exception
- **THEN** its pending image payloads and resolved values are not retained by the next scope
