# Spec Delta

## ADDED Requirements

### Requirement: An image output may be a two-dimensional map

A model's `image:` block SHALL be able to declare an output whose tensor is a two-dimensional map — a rank-3 `[1, H, W]` or rank-4 `[1, 1, H, W]` tensor — through `select: map`. Such an output SHALL be emitted as `H × W` packed `float32` in row-major order and SHALL report `rows: H` and `dim: W` in `EMB.INFO`. It MUST NOT be normalized, pooled, sliced or otherwise reduced.

#### Scenario: A depth map reaches the client

- **WHEN** a model declares `{name: depth, tensor: predicted_depth, select: map}` and an image is sent
- **THEN** the reply carries `H × W` `float32` values in row-major order, and `EMB.INFO` reports the output's `select: map`, `rows: H` and `dim: W`

#### Scenario: The map is not reduced

- **WHEN** a map contains negative values, zeros, or values far outside a unit range
- **THEN** they arrive unchanged in value and order

#### Scenario: A map's shape is declared, not guessed

- **WHEN** a client reads the model's image outputs before calling
- **THEN** it can compute the reply's element count as `rows × dim` and refuse to draw when the reply disagrees

#### Scenario: Map configuration errors fail at load

- **WHEN** a map output also declares `start` or `grid`, or names a tensor whose rank is not 3 or 4
- **THEN** model loading fails with an error naming the output and the reason

#### Scenario: A map has no text reply

- **WHEN** a client asks for a map output with `VALUES`
- **THEN** the server refuses with an error naming the `BLOB` form, and performs no inference

#### Scenario: The map is cached by content

- **WHEN** the same image is sent twice for the same map output
- **THEN** the second request hits the content-addressed image cache and the bytes are identical
