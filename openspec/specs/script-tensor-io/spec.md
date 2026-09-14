## MODIFIED Requirements

### Requirement: Packed tensor outputs

`emb.run` and `emb.run_batch` SHALL accept an options table as their final argument, and when it requests packed output (`{bytes = true}`) each returned tensor SHALL carry its data as a Lua string of little-endian raw elements instead of a per-element array:

```
{ shape = {1, 512, 384}, bytes = <string>, dtype = "f32" | "i64" }
```

The packed representation SHALL be byte-compatible with the `bytes` input field of `emb.run` and with `emb.math.float32_bytes` output, and SHALL round-trip: a packed output passed back as a `bytes` input SHALL reproduce the same tensor. The default result form SHALL be the per-element `data` array together with `shape` and the tensor's `dtype` (`{shape, data, dtype}`), so every returned tensor is already a valid `emb.run` input spec and no dtype is re-inferred from element values. Scripts that read only `data` are unaffected.

#### Scenario: Packed output round-trips through an input

- **WHEN** a script requests `{bytes = true}` from `emb.run` and feeds the returned `bytes` and `shape` back into `emb.run` as an input tensor
- **THEN** the second run produces the same output as the first

#### Scenario: Default form is unchanged

- **WHEN** a script calls `emb.run` without an options table
- **THEN** outputs carry a `data` array exactly as before

#### Scenario: Default form carries dtype

- **WHEN** a script calls `emb.run` without an options table
- **THEN** each output also carries its `dtype`, and feeding `{shape, data, dtype}` back into `emb.run` preserves the tensor dtype even when every element is integral

#### Scenario: Packed length matches shape

- **WHEN** a packed output is returned
- **THEN** its byte length equals the product of its shape times the element width (4 for `f32`, 8 for `i64`)

### Requirement: Vector math over packed buffers

The server SHALL provide `emb.math` operations that accept either a per-element array or a packed `bytes` string and compute in the host runtime without building intermediate Lua tables:

- `dot(a, b)` — inner product
- `cosine(a, b)` — cosine similarity
- `l2(a, b)` — Euclidean distance
- `norm(a)` — L2 norm

Two operands SHALL be validated for equal element counts; a packed operand whose length is not a multiple of 4 SHALL be an error. An operand with zero elements SHALL be accepted where the operation has a defined empty result: `dot`, `l2`, and `norm` SHALL return `0`, while `cosine` (whose denominator is zero) SHALL error. `sigmoid`, `softmax`, and `argmax` SHALL additionally accept packed `bytes` operands with the same semantics they have for arrays.

#### Scenario: Packed and array operands agree

- **WHEN** the same vector is passed to `emb.math.cosine` once as an array and once packed
- **THEN** both calls return the same value within float tolerance

#### Scenario: Sigmoid accepts packed input

- **WHEN** `emb.math.sigmoid` is called with a packed float32 buffer
- **THEN** the result is the array of element-wise sigmoid values

#### Scenario: Malformed packed buffer errors

- **WHEN** a packed operand's byte length is not a multiple of 4
- **THEN** the evaluation fails with a length error

#### Scenario: Empty linear operands yield zero

- **WHEN** `emb.math.dot({}, {})`, `emb.math.l2({}, {})`, or `emb.math.norm({})` is called
- **THEN** the result is `0`

#### Scenario: Empty cosine errors

- **WHEN** `emb.math.cosine({}, {})` is called
- **THEN** the evaluation fails with an error reply

### Requirement: Pooling and reduction operations

The server SHALL provide host-side reductions over packed or array tensors so scripts do not re-implement them in interpreted Lua:

- `mean_pool(hidden, shape, mask)` — masked mean over the sequence axis followed by L2 normalization, returning one vector per batch row. `shape` is `{batch, seq, dim}`; `mask` is a flat `batch × seq` array of 0/1 values.
- `cls(hidden, shape)` — the first sequence position of each batch row, followed by L2 normalization.
- `topk(values, k)` — the `k` highest values as `{index, value}` pairs in descending value order.
- `slice(tensor, shape, offset, length)` and `gather(values, indices)`.

Operations SHALL validate that `shape` agrees with the operand's element count and SHALL error otherwise. Scalar arguments (`k`, `offset`, `length`, and every element of `indices`) SHALL be integers: a fractional value SHALL error rather than being truncated to an integer. An empty selection SHALL return an empty array: `topk` of an empty tensor, `gather` with an empty index list, and `slice` of zero length all yield `{}`. All reductions SHALL be deterministic so scripted replies remain cacheable.

#### Scenario: Masked mean matches the embedding pipeline

- **WHEN** a script mean-pools a hidden-state tensor with a mask using `emb.math.mean_pool` and the same tensor is pooled by the embedding path
- **THEN** the resulting vectors agree within float tolerance

#### Scenario: Mask excludes padded positions

- **WHEN** the mask is zero for trailing positions
- **THEN** those positions do not contribute to the mean

#### Scenario: topk returns descending order

- **WHEN** `emb.math.topk(values, k)` is called
- **THEN** the result lists the `k` largest values in descending order, each with its 1-based index

#### Scenario: Shape mismatch errors

- **WHEN** `shape` does not match the operand's element count
- **THEN** the evaluation fails with an error naming both

#### Scenario: Fractional arguments are rejected

- **WHEN** a script calls `emb.math.gather({10, 20}, {1.5})`, `emb.math.slice({1, 2, 3}, {3}, 1.5, 2)`, or `emb.math.topk({1, 2, 3}, 2.5)`
- **THEN** the evaluation fails with an error rather than silently truncating the value

#### Scenario: Empty selection returns an empty array

- **WHEN** a script calls `emb.math.topk({}, 3)` or `emb.math.gather({1, 2, 3}, {})`
- **THEN** the result is an empty array
