# Spec Delta

## ADDED Requirements

### Requirement: The operations section documents the fleet pairing with its measured figure

The docs surface's operations section SHALL document `emb-stress` beside
`emb-top`: what the run takes as input, what it offers, what it reports per
endpoint and for the fleet, and that reading the two tools together is what
separates the share a node was offered from the share it served. It SHALL carry
the measured figure with the invocation that reproduced it. No new top-level
section SHALL be introduced.

#### Scenario: The invocation is documented

- **WHEN** a reader opens the operations section
- **THEN** it states the run's own form — a server configuration naming the models exercised, a target that is one address or a DNS name expanded into the node set, and the levels and dwell the run used — with the command that reproduces it

#### Scenario: The pairing is stated

- **WHEN** the section describes what the two tools report
- **THEN** it states that `emb-stress` offers and counts the load while `emb-top` observes the fleet, and that a node offered its share that serves less is the node to look at first

#### Scenario: The figure's numbers carry provenance

- **WHEN** the docs surface presents a value the figure draws
- **THEN** that value appears in `BENCHMARK.md` with the machine, the fleet and the reproduction command that produced it

#### Scenario: No new section is introduced

- **WHEN** the operations section is reviewed
- **THEN** the fleet documentation and its figure sit inside the existing operations section, and the surface's section order is unchanged
