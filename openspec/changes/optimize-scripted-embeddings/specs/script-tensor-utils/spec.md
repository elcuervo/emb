## Purpose

Gives scripts constant-filled tensor construction without Lua table round-trips (`fill`) and raw float32 byte packing (`emb.math.float32_bytes`), so scripted embeddings return the same byte layout as the embed path and avoid per-element RESP blowup.

## ADDED Requirements

### Requirement: Constant-filled tensor specs (`fill`)

The server SHALL accept a `fill` field on `emb.run` and `emb.run_batch` input specs as an alternative to `data`: `{shape = {...}, fill = n, dtype = "i64"|"f32"}` SHALL construct a tensor of the given shape with every element equal to `n`, allocated host-side without a Lua data table. `fill` and `data` SHALL be mutually exclusive (an error when both are present); `dtype` behavior matches the existing spec rules (explicit or inferred — a non-integer `fill` infers float32).

#### Scenario: Zero-filled float tensor for a fused model's auxiliary input

- **WHEN** a script feeds an auxiliary float input with `{shape = {1, 3, 224, 224}, fill = 0, dtype = "f32"}`
- **THEN** the session receives a float32 tensor of that shape filled with zeros, constructed without a Lua data table

#### Scenario: Ones-filled mask tensor

- **WHEN** a script feeds `{shape = {1, 8}, fill = 1, dtype = "i64"}`
- **THEN** the session receives an int64 tensor with eight ones

#### Scenario: Fill and data conflict errors

- **WHEN** a spec declares both `fill` and `data`
- **THEN** the evaluation fails with an error reply

### Requirement: Raw float32 byte packing (`emb.math.float32_bytes`)

The server SHALL provide `emb.math.float32_bytes(vals)`: an array of Lua numbers SHALL be packed into a Lua string of 4 little-endian float32 bytes per element (IEEE 754). The reply grammar already passes strings through as bulks, so `return emb.math.float32_bytes(embedding)` yields a single bulk byte-identical to the embed path's float32 replies; clients decode with `unpack('e*')`. Empty arrays SHALL error.

#### Scenario: Vector reply as a single byte bulk

- **WHEN** a script returns `emb.math.float32_bytes(vec)` for a 768-dim vector
- **THEN** the reply is one bulk string of 3072 bytes decodable back to the original 768 float32 values

#### Scenario: Empty array errors

- **WHEN** `emb.math.float32_bytes({})` is called
- **THEN** the evaluation fails with an error reply