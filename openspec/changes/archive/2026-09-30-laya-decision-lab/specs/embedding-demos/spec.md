## ADDED Requirements

### Requirement: A decision demo draws each example's typed questions as a tree

A demo plate for a decision model SHALL teach the model's typed-question
vocabulary before showing a call, and SHALL draw the question set it is about to
send as a tree: the state as the root, one branch per question carrying its
question id and type, and one leaf per criterion or level. The leaves SHALL be
filled with the probabilities from the reply the server returned — the tree MUST
NOT be a hand-authored illustration of a result, and every example on the plate
MUST render through the same figure.

#### Scenario: The tree is the payload, filled by the reply

- **WHEN** an example runs
- **THEN** the drawn tree's branches and leaves are the questions and criteria of the request, and each leaf's value is the corresponding probability in the reply

#### Scenario: The vocabulary is taught before the call

- **WHEN** the plate is read from the top
- **THEN** `choice`, `score`, and `noul` are each defined with the request they take and the reply shape they return before any example runs

#### Scenario: Multiple examples share one interaction

- **WHEN** a visitor switches between the plate's examples
- **THEN** each one runs through the same call, the same tree figure, and the same command disclosure
