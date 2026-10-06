# Spec Delta

## ADDED Requirements

### Requirement: emb-top pins each labelled metric's label column

`emb-top` SHALL render every labelled per-model metric (`p50`, `p95`, the `avg`
fallback and `err`) as a fixed label followed by its value, so the label keeps
its column whatever the value's width and whichever metric the row is showing. A
model with no recent requests SHALL therefore align its `avg` label with a busy
model's `p50` label.

#### Scenario: An idle row lines up with a busy row

- **WHEN** one model renders `avg` from its lifetime average and another renders `p50` from recent events
- **THEN** both labels start at the same column, and their `err` labels do too

#### Scenario: A growing value moves no label

- **WHEN** a metric's value grows between polls (for example `3µs` to `16.9ms`)
- **THEN** its label and the labels after it keep their columns
