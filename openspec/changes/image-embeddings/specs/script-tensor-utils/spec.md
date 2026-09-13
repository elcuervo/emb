## ADDED Requirements

### Requirement: Packed-byte tensor input (`bytes`)

The server SHALL accept a `bytes` field on `emb.run` and `emb.run_batch` input specs as a third alternative to `data` and `fill`: `{shape = {...}, bytes = <string>, dtype = "f32"|"i64"}` SHALL interpret the string as the tensor's raw little-endian elements (4 bytes per element, `float32` for `f32`, 8 bytes per element, `int64` for `i64`). This is the inverse of `emb.math.float32_bytes` and lets host-produced tensors (for example `emb.image.preprocess` output) reach the session without a per-element Lua table round-trip. Exactly one of `data`, `fill`, or `bytes` SHALL be provided. The byte length SHALL exactly match `elementCount × elementWidth`; a mismatch SHALL be an error before inference. `dtype` SHALL be required for `bytes` (no inference from content). Packed tensors SHALL be charged against the per-tensor and request-wide element budgets exactly like `data` tensors.

#### Scenario: Packed float tensor feeds the session

- **WHEN** a spec is `{shape = {1, 3, 224, 224}, bytes = <602112 bytes>, dtype = "f32"}`
- **THEN** the session receives a float32 tensor of that shape with no per-element Lua table construction

#### Scenario: Round-trip with float32_bytes

- **WHEN** a tensor's values are packed with `emb.math.float32_bytes` and fed back via `bytes` with the same shape
- **THEN** the session receives the original values

#### Scenario: Length mismatch errors

- **WHEN** `bytes` length does not equal `elementCount × elementWidth` for the declared shape and dtype
- **THEN** the evaluation fails with an error before inference

#### Scenario: Mutually exclusive with data and fill

- **WHEN** a spec provides more than one of `data`, `fill`, and `bytes`, or omits all three
- **THEN** the evaluation fails with an error reply
