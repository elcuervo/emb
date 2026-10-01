# Spec Delta

## ADDED Requirements

### Requirement: Inference duration returned by emb.run

`emb.run` and `emb.run_batch` SHALL return, in addition to the map of output
tensors, the model call's own inference duration in milliseconds as a second
return value. The duration SHALL be measured host-side around the single session
run that produces the outputs, so it covers the graph call and not the tensor
marshalling, tokenization, or other script work around it. A script that reads only
the first return value SHALL behave exactly as before.

#### Scenario: A script captures the inference duration

- **WHEN** a script calls `emb.run` and captures its second return value
- **THEN** the captured value is a finite number of milliseconds, greater than zero for a successful run

#### Scenario: Ignoring the second value is unchanged

- **WHEN** a script assigns only `emb.run`'s first return value, as scripts written before this change do
- **THEN** the call produces the same output tensors as it did before, with no error

#### Scenario: A batch reports one duration

- **WHEN** a script calls `emb.run_batch` for several rows
- **THEN** the second return value is a single duration for the whole batch call, not one per row

#### Scenario: The duration is the model call, not the request

- **WHEN** the same script also reports a request-level elapsed time
- **THEN** the inference duration it returns is smaller than, and independent of, the request time, because it measures only the session run
