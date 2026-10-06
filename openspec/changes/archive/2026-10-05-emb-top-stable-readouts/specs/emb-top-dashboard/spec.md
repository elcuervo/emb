# Spec Delta

## ADDED Requirements

### Requirement: emb-top keeps its readouts on fixed columns

`emb-top` SHALL render every dynamic numeric readout — the health banner chips,
the header's uptime and model count, the gauge captions, the connection line and
the event ticker — in a fixed-width column, so a value growing under load moves
no glyph and no text after it. The health status label SHALL occupy a fixed width
so a verdict change does not shift the chips that follow it.

#### Scenario: Banner chips do not shuffle under load

- **WHEN** the error ratio, p95 latency, CPU and cache values grow between polls
- **THEN** every banner chip keeps its column and no chip after a growing value moves

#### Scenario: Gauge values keep their column

- **WHEN** a gauge's value grows (for example `0.0%` to `63.9%`)
- **THEN** the number and its unit stay in the same columns

#### Scenario: A status change does not shift the chips

- **WHEN** the banner's verdict changes between statuses of different text width
- **THEN** the chips after the verdict keep their columns

#### Scenario: A narrow band shrinks the value column

- **WHEN** a gauge's band is too narrow for the full-width value column
- **THEN** the value column shrinks with the band and the line does not overflow the terminal
