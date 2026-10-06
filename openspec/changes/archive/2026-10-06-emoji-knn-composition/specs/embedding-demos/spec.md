# Spec Delta

## MODIFIED Requirements

### Requirement: The plate's examples are a fixed, verified set

The plate SHALL offer a set of example queries covering each mode the zone
answers — a sentence, an emoji, an arithmetic composition, and a conjunction —
each reachable from one control and each named by the URL fragment. Every
offered example MUST be one of the zone's shipped, verified examples, so the
control set is the pinned measurement rather than a second copy of it, and the
plate SHALL state which mode each example demonstrates.

#### Scenario: One control per mode

- **WHEN** the plate is rendered
- **THEN** a reader can run a sentence example, an emoji example, an arithmetic composition, and a conjunction from one control each

#### Scenario: An example is named by the URL

- **WHEN** a reader selects an example
- **THEN** the fragment names it, and opening that fragment selects it again

#### Scenario: A shipped example is verified, not asserted

- **WHEN** the examples are checked before deployment
- **THEN** each one returns the result the plate's own copy claims for it, and a drifting example fails the check rather than shipping

#### Scenario: A newly pinned joke is a control

- **WHEN** an example is added to the zone's shipped examples and the plate is rebuilt
- **THEN** the plate offers it as a control without an edit to the plate's content

## ADDED Requirements

### Requirement: The plate can be played with, not only queried

A reader SHALL be able to build a query from what the plate has already shown:
any ranked result the plate renders SHALL be addable to the query as a term, and
the operator between terms SHALL be selectable. The name the plate queries, the
`dig` invocation it displays, and the query it reports MUST all follow the
composition the reader built.

#### Scenario: A result becomes a term

- **WHEN** a reader adds a ranked result to the query
- **THEN** that result's glyph is a term of the name the plate queries

#### Scenario: The operator is the reader's choice

- **WHEN** a reader selects an operator for the term they add
- **THEN** the query the plate sends carries that operator, and the plate sends nothing the zone would refuse

#### Scenario: The built query is the displayed query

- **WHEN** the plate renders an answer to a composed query
- **THEN** the displayed name, the displayed `dig` invocation, and the reported query all carry the same composition

### Requirement: The plate states what each operator asks

Beside the controls that compose a query, the plate SHALL state what each
operator asks for, so a reader can tell an arithmetic composition from a
conjunction without reading the zone's documentation.

#### Scenario: The operators are explained

- **WHEN** a reader is shown the controls that compose a query
- **THEN** the plate states that `+` and `-` compose vectors while `*` asks for the entry closest to all of its terms

### Requirement: A conjunction's answer shows why its terms agree

When the plate renders a conjunction, each shown result SHALL carry its
similarity to each of the query's terms beside its joint score, so a reader can
tell a joke from a stretch. A conjunction whose terms barely agree MUST NOT be
presented as confidently as one whose terms agree strongly, and the plate SHALL
state that a low joint score is the honest reading rather than a fault.

#### Scenario: The legs are shown

- **WHEN** the plate renders the results of a conjunction
- **THEN** each result shows its similarity to each of the query's terms and its joint score

#### Scenario: A weak conjunction reads as weak

- **WHEN** a conjunction's results carry low joint scores
- **THEN** the plate says so instead of presenting them as a confident answer

#### Scenario: A joke can be explained

- **WHEN** a reader asks why a conjunction answered with the entry it did
- **THEN** the shown similarities explain it — which terms the entry is close to, and which it is not
