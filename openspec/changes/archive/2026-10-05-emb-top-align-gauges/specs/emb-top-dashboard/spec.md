# Spec Delta

## ADDED Requirements

### Requirement: emb-top aligns its gauges to the per-model row grid

`emb-top` SHALL lay the cache, CPU and memory gauges on the same column grid as
the per-model rows: the cache gauge under the identity block, the CPU gauge under
the metric block, and the memory gauge under the activity strip, each bar
spanning the columns of the zone above it. The grid SHALL be a function of the
terminal width alone, so it does not shift between rows or between polls.

#### Scenario: Gauges share the row's columns

- **WHEN** the dashboard renders per-model rows and the gauges below them
- **THEN** each gauge bar starts at the same column as the row zone it sits under and spans that zone's width

#### Scenario: The grid does not move when latency events lapse

- **GIVEN** a model rendering its p50/p95 columns and a model falling back to its average latency
- **WHEN** both rows repaint on the same poll
- **THEN** their activity strips start at the same column and the gauges stay on the same grid

#### Scenario: Narrow terminal drops columns without breaking the grid

- **WHEN** the terminal is too narrow for every metric column
- **THEN** the grid drops trailing metric columns, and the rows and gauges still share one grid without overflowing the terminal width
