## ADDED Requirements

### Requirement: Element-wise arithmetic (`emb.math.scale`, `emb.math.add`)

The server SHALL provide `emb.math.scale(values, factor)`, multiplying every element by `factor`, and `emb.math.add(a, b)`, the element-wise sum of two operands. Each SHALL accept a per-element array or a packed little-endian float32 string and SHALL return a per-element array of numbers. `add` SHALL require the two operands to have equal element counts and SHALL error on a mismatch. An operand with zero elements SHALL yield an empty array (`scale({}, f)` and `add({}, {})` both return `{}`), consistent with the other element-wise map (`sigmoid`); a length mismatch still errors.

#### Scenario: Scale multiplies every element

- **WHEN** a script calls `emb.math.scale({1, 2, 3}, 2)`
- **THEN** the result is `{2, 4, 6}`

#### Scenario: Add sums matching operands

- **WHEN** a script calls `emb.math.add({1, 2}, {3, 4})`
- **THEN** the result is `{4, 6}`

#### Scenario: Length mismatch errors

- **WHEN** a script calls `emb.math.add({1, 2}, {3})`
- **THEN** the evaluation fails with a length error

#### Scenario: Empty operand returns an empty array

- **WHEN** a script calls `emb.math.scale({}, 2)` or `emb.math.add({}, {})`
- **THEN** the result is an empty array
