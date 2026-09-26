## ADDED Requirements

### Requirement: A demo shows one forward pass answering many typed questions

The gallery SHALL include a plate that demonstrates the decision surface: one scripted call to a decision model that answers several typed questions at once, and whose rendering makes the single-forward-pass fact legible (the answers SHALL appear together, as one reply, rather than one after another). The plate SHALL use the shared demo anatomy — plate, mechanism, rig, commands-that-unfold — and SHALL ship a preselected set of example states, each reusable from one button.

#### Scenario: One run answers every question

- **WHEN** a visitor selects an example state and runs the plate
- **THEN** exactly one `EMB.EVSHA <model> <digest> 1 <state> <questions> <config>` evaluation issues, and every question's answer appears at once in the same rendering pass

#### Scenario: The answers differ in shape by question type

- **WHEN** the plate renders a `choice`, a `score`, and a `noul` answer
- **THEN** the `choice` shows a distribution with a winner, the `score` shows a position on a legend, and the `noul` shows a probability, and every answer carries its confidence (and the sheet carries the action probability)

### Requirement: A stand-in model demo states that its numbers are not judgments

A demo whose live model is a mechanism substitute — same architecture and export pipeline, weights that answer nothing — SHALL say so in the plate's prose and figure, SHALL NOT present its numbers as conclusions about the example states, and SHALL show what the production-shaped call looks like (the same command, arguments, and config envelope against the real checkpoint) as the integration artifact.

#### Scenario: The substitute is labeled

- **WHEN** a plate runs against a stand-in model
- **THEN** the plate states that the toy model demonstrates the mechanism only, that its answers are shapes rather than judgments, and that the same command with the production checkpoint is the documented integration

#### Scenario: The production call is shown

- **WHEN** the plate's exact-commands section presents the stand-in run
- **THEN** it also shows the production-shaped invocation (state, questions, config envelope) that a deployment uses against the published checkpoints