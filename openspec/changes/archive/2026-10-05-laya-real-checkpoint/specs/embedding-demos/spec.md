# Spec Delta

## MODIFIED Requirements

### Requirement: A stand-in model demo states that its numbers are not judgments

A demo whose live model is a mechanism substitute — same architecture and export
pipeline, weights that answer nothing — SHALL say so in the plate's prose and figure,
SHALL NOT present its numbers as conclusions about the example states, and SHALL
show what the production-shaped call looks like. A plate that answers some examples
with a real checkpoint and others with a stand-in SHALL label each example's model,
so a reader can tell which numbers are judgments and which are only the mechanism.
The stand-in rule SHALL NOT be applied to an example answered by a real checkpoint.

#### Scenario: The substitute is labeled

- **WHEN** a plate runs an example against a stand-in model
- **THEN** the plate states that the model demonstrates the mechanism only and that its answers are shapes rather than judgments

#### Scenario: A real example is not labeled a substitute

- **WHEN** an example is answered by a real checkpoint
- **THEN** the plate does not present it as a mechanism substitute

#### Scenario: The production call is shown

- **WHEN** the plate's exact-commands section presents an example
- **THEN** it shows the invocation (model entry, state, questions, config envelope) that produced the displayed reply
