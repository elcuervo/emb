## ADDED Requirements

### Requirement: Boolean tensors in named-tensor IO

`emb.run` input and output tensors SHALL support a `bool` dtype in addition to `i64` and `f32`. A Lua input tensor may declare `dtype = "b1"` (or `"bool"`) with a per-element `data` array of 0/1 integers or a packed `bytes` string of 1-byte elements; a returned tensor SHALL carry `dtype = "b1"` and SHALL round-trip through a subsequent `emb.run` input. ONNX bool graph inputs (e.g. Laya's `marker_mask`) SHALL be feedable and a graph bool output SHALL be readable; mismatched dtypes SHALL produce a run error, never a silent cast.

#### Scenario: Bool input feeds an ONNX bool graph input

- **WHEN** a script passes `{shape = {1, 2}, data = {1, 0}, dtype = "b1"}` for a graph input declared as ONNX bool
- **THEN** the run succeeds and the model receives the corresponding boolean values

#### Scenario: Bool output round-trips

- **WHEN** a run returns a bool tensor and the script feeds its `{shape, data, dtype}` back as an input
- **THEN** the second run accepts it and produces the same output

#### Scenario: Wrong dtype for a bool input errors

- **WHEN** a script feeds an `i64` or `f32` tensor where the graph declares a bool input
- **THEN** the run fails with an error and no inference result is returned