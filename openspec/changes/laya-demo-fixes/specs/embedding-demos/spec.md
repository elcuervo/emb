# Spec Delta

## ADDED Requirements

### Requirement: A re-selected demo replaces its surface

When a visitor re-selects any control that changes which example or item a demo is
showing, the demo SHALL replace its surface rather than append to it. A single
selection MUST NOT leave two copies of a control in the DOM, and the number of
rendered controls MUST NOT grow with the number of selections.

#### Scenario: Switching an Inbox ticket

- **WHEN** a visitor selects a different Inbox ticket
- **THEN** the ticket list and its form appear exactly once, not once per selection

#### Scenario: Repeated switches stay single

- **WHEN** a visitor switches between tickets several times
- **THEN** each switch leaves exactly one list and one form, regardless of how many switches happened

### Requirement: A looped demo's readout shows the executed decision

A looped demo's readout SHALL lead with the move the preset actually executed, and
SHALL show the model's move probabilities over the **legal** moves only — a
direction the rules forbid MUST be shown as unavailable, not as a probability. The
executed move SHALL be marked in the probability table, and the model's own
proposal SHALL be named beside it whether or not the safety layer overrode it.

#### Scenario: Probabilities cover the legal moves only

- **WHEN** a looped demo displays a decision and some directions are blocked by the rules
- **THEN** only the legal directions carry a probability, and a blocked direction shows no value

#### Scenario: The executed move is the highlight

- **WHEN** a looped demo displays a decision
- **THEN** the executed move is named as the decision and marked in the table, and the model's proposal is shown separately

#### Scenario: A veto is visible

- **WHEN** the safety layer overrides the model's proposal
- **THEN** the readout names the executed move, names the proposal it replaced, and marks the intervention

### Requirement: The readout does not reflow as its values change

A demo readout SHALL keep its layout as its values change. A longest permitted
label or marker MUST NOT wrap to a second line or push a value out of its column,
and a row's height and alignment MUST NOT change when a different value is
displayed.

#### Scenario: A long direction is proposed

- **WHEN** the proposed or executed move is the longest direction name
- **THEN** the move's marker, name, bar and value stay on one aligned row

#### Scenario: The readout-bar labels fit

- **WHEN** the readout's bar labels are drawn
- **THEN** each label stays on one line and its bar and value remain aligned to it
