## Purpose

Lets scripts move tensor data in and out of the sandbox in packed form and reduce it with host-side operations, so large model outputs are never materialized element-by-element into Lua and never processed with interpreted loops.

## ADDED Requirements

### Requirement: Packed tensor outputs

`emb.run` and `emb.run_batch` SHALL accept an options table as their final argument, and when it requests packed output (`{bytes = true}`) each returned tensor SHALL carry its data as a Lua string of little-endian raw elements instead of a per-element array:

```
{ shape = {1, 512, 384}, bytes = <string>, dtype = "f32" | "i64" }
```

The packed representation SHALL be byte-compatible with the `bytes` input field of `emb.run` and with `emb.math.float32_bytes` output, and SHALL round-trip: a packed output passed back as a `bytes` input SHALL reproduce the same tensor. The default result form SHALL remain the per-element `data` array, so existing scripts are unaffected.

#### Scenario: Packed output round-trips through an input

- **WHEN** a script requests `{bytes = true}` from `emb.run` and feeds the returned `bytes` and `shape` back into `emb.run` as an input tensor
- **THEN** the second run produces the same output as the first

#### Scenario: Default form is unchanged

- **WHEN** a script calls `emb.run` without an options table
- **THEN** outputs carry a `data` array exactly as before

#### Scenario: Packed length matches shape

- **WHEN** a packed output is returned
- **THEN** its byte length equals the product of its shape times the element width (4 for `f32`, 8 for `i64`)

### Requirement: Selective outputs

When the options table names outputs (`{outputs = {"name", ...}}`), the server SHALL return and materialize only those outputs; unnamed graph outputs SHALL NOT be converted. Naming an output the graph does not produce SHALL be an error listing the available outputs. Omitting the field SHALL preserve today's behaviour of returning every output.

#### Scenario: Only requested outputs are materialized

- **WHEN** a script requests `{outputs = {"logits"}}` from a graph that also produces a large unused tensor
- **THEN** the result contains only `logits`, and the unused output is not converted for the sandbox

#### Scenario: Unknown output name errors

- **WHEN** a script requests an output name the graph does not produce
- **THEN** the evaluation fails with an error naming the requested and available outputs

#### Scenario: Applies to batched runs

- **WHEN** `emb.run_batch` is called with an options table selecting outputs
- **THEN** every per-item result contains only the requested outputs

### Requirement: Vector math over packed buffers

The server SHALL provide `emb.math` operations that accept either a per-element array or a packed `bytes` string and compute in the host runtime without building intermediate Lua tables:

- `dot(a, b)` — inner product
- `cosine(a, b)` — cosine similarity
- `l2(a, b)` — Euclidean distance
- `norm(a)` — L2 norm

Operands SHALL be validated for equal element counts; a packed operand whose length is not a multiple of 4 SHALL be an error. `sigmoid`, `softmax`, and `argmax` SHALL additionally accept packed `bytes` operands with the same semantics they have for arrays.

#### Scenario: Packed and array operands agree

- **WHEN** the same vector is passed to `emb.math.cosine` once as an array and once packed
- **THEN** both calls return the same value within float tolerance

#### Scenario: Sigmoid accepts packed input

- **WHEN** `emb.math.sigmoid` is called with a packed float32 buffer
- **THEN** the result is the array of element-wise sigmoid values

#### Scenario: Malformed packed buffer errors

- **WHEN** a packed operand's byte length is not a multiple of 4
- **THEN** the evaluation fails with a length error

### Requirement: Pooling and reduction operations

The server SHALL provide host-side reductions over packed or array tensors so scripts do not re-implement them in interpreted Lua:

- `mean_pool(hidden, shape, mask)` — masked mean over the sequence axis followed by L2 normalization, returning one vector per batch row. `shape` is `{batch, seq, dim}`; `mask` is a flat `batch × seq` array of 0/1 values.
- `cls(hidden, shape)` — the first sequence position of each batch row, followed by L2 normalization.
- `topk(values, k)` — the `k` highest values as `{index, value}` pairs in descending value order.
- `slice(tensor, shape, offset, length)` and `gather(values, indices)`.

Operations SHALL validate that `shape` agrees with the operand's element count and SHALL error otherwise. All reductions SHALL be deterministic so scripted replies remain cacheable.

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

### Requirement: Packed I/O is budgeted

Packed outputs SHALL count their element count against the same per-evaluation tensor budget as array outputs, and the total bytes materialized per evaluation SHALL be bounded. Exceeding a bound SHALL fail that evaluation only, leaving other in-flight requests unaffected.

#### Scenario: Oversized packed output is rejected

- **WHEN** a script requests a packed output whose element count exceeds the evaluation's tensor budget
- **THEN** the evaluation fails with a budget error and no partial reply is written

#### Scenario: Budget counts packed and array forms equally

- **WHEN** the same tensor is requested as an array and as packed
- **THEN** both forms consume the same element budget
