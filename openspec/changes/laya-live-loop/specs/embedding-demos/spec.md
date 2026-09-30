## ADDED Requirements

### Requirement: A dependent decision loop runs as one bounded episode

A demo whose decisions form a loop — each step depending on the previous one —
SHALL run that loop where the model runs, as a **bounded episode inside a single
call**, and SHALL animate the returned trace locally. It MUST NOT require a
network round trip per rendered frame, and it MUST NOT present a precomputed or
page-authored trace: every frame SHALL be a decision the server computed during
that call. The episode SHALL be bounded so a single request cannot run unbounded,
and the demo SHALL keep a single-step path so the per-decision mechanism remains
inspectable.

#### Scenario: One call returns the episode

- **WHEN** a visitor starts the looped demo
- **THEN** one command returns the episode's frames, and the commands disclosure shows that single call rather than one call per frame

#### Scenario: The animation survives a slow network

- **WHEN** frames are playing and the buffer runs low
- **THEN** the demo requests the next episode from the last frame's state before the buffer empties, so the visible animation does not stall on a round trip

#### Scenario: Frames are the server's

- **WHEN** a frame is drawn
- **THEN** its board and probabilities came from the server's reply for that episode, and a failed fetch shows the sandbox's state instead of any frame

#### Scenario: The single step remains

- **WHEN** a visitor steps once
- **THEN** exactly one tick is computed in one call and its typed questions and probabilities are shown for that single decision

#### Scenario: The episode is bounded

- **WHEN** a request asks for more ticks than the documented bound
- **THEN** the request is rejected or clamped rather than running an unbounded loop
