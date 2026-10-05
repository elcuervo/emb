# Spec Delta

## ADDED Requirements

### Requirement: A decision readout shows per-decision model inference time

A decision demo's readout SHALL show the model's own inference time for the
decision it is displaying, not the request round trip. The value SHALL come from
the server's measurement of the model call, and the plate MUST NOT substitute a
client clock around the request or the bridge's whole-command elapsed time. For a
looped demo the readout SHALL show the displayed frame's inference time; for a
single-pass demo it SHALL show the reply's inference time. The plate SHALL label
the value as model inference and, where the figure reads as a rate, derive that
rate from the inference time rather than from wall-clock playback.

#### Scenario: The looped readout shows the frame's inference time

- **WHEN** a looped decision demo displays a frame
- **THEN** the readout's inference figure is that frame's server-measured model time, and it changes as frames advance

#### Scenario: The single-pass readout shows the reply's inference time

- **WHEN** a typed-question demo shows a completed reply
- **THEN** the readout's inference figure is the reply's server-measured model time

#### Scenario: The figure is not the request time

- **WHEN** a visitor inspects the readout's inference value
- **THEN** it is the model call's duration, and the plate does not present the client-measured request duration as that figure

### Requirement: The typed-question tree fills as the reply arrives

The decision tree SHALL fill its leaves from the reply's probabilities as an
ordered reveal rather than a single instantaneous frame, so a visitor sees each
question's distribution populate in turn. The reveal SHALL be presentational only:
every leaf's final value MUST equal the reply's probability, the tree MUST be
complete in one frame under reduced motion, and no value may be shown before the
reply that carries it exists.

#### Scenario: Leaves populate in question order

- **WHEN** a typed-question reply arrives
- **THEN** each question's leaves fill in turn, and each leaf ends at the probability the reply carries for it

#### Scenario: Reduced motion keeps the whole tree

- **WHEN** a reader prefers reduced motion
- **THEN** the tree is drawn complete in one frame and no leaf depends on the reveal having run

#### Scenario: No fabricated value before the reply

- **WHEN** the plate draws the tree before the reply returns
- **THEN** every leaf is empty and no probability is shown

### Requirement: A looped demo's planner prevents stalls

A looped demo whose decisions move a position SHALL NOT allow the run to oscillate
without progress. Its planner SHALL veto an immediate reversal of the previous move
when another legal move exists, and SHALL veto a move that increases the distance
to the nearest objective when a move that decreases it exists. After a documented
number of consecutive decisions without collecting an objective, the planner SHALL
take the objective-seeking move until progress resumes.

#### Scenario: An immediate reversal is vetoed

- **WHEN** the model proposes a reversal of the previous move and another legal move exists
- **THEN** the executed move is not that reversal

#### Scenario: A regressing move is vetoed

- **WHEN** the model proposes a move that increases the distance to the nearest objective while a move that decreases it is legal
- **THEN** the executed move does not increase that distance

#### Scenario: A stalled run recovers

- **WHEN** a documented number of decisions pass without an objective being collected
- **THEN** the next executed move reduces the distance to the nearest objective until one is collected

#### Scenario: A bounded episode makes progress

- **WHEN** a bounded episode runs from a fresh state
- **THEN** it collects at least one objective, or the objective set was already empty
