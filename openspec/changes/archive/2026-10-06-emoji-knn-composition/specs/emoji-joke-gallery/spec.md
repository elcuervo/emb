# Spec Delta

## Purpose

Keeps the zone's shipped examples honest: each one is a measurement of what the
running zone answers, discovered from the model rather than asserted, and gated
against drift so a changed model, vocabulary, or operator is a decision someone
makes instead of a copy that quietly stops being true.

## ADDED Requirements

### Requirement: A shipped example records the answer that was measured

Every shipped example SHALL record the query, the ranked answer the zone
returned when the example was pinned — at least the winning glyph, its
vocabulary name, and its similarity score — and the answer the query's own shape
produces, such as an emoji sentence. An example MUST NOT ship an answer that was
not measured, and gallery copy that explains an example MUST describe the
recorded answer rather than an intended one.

#### Scenario: An example carries its measured answer

- **WHEN** a shipped example is read
- **THEN** it carries the query, the winning glyph, its name, its score, and any emoji sentence the query produced

#### Scenario: Copy cannot claim an answer the zone does not give

- **WHEN** an example's explanation and its recorded answer are compared
- **THEN** the explanation describes the recorded answer

### Requirement: The shipped examples are gated against drift

A check SHALL ask the running zone every shipped example again and SHALL fail
when a recorded glyph, name, or sentence no longer matches, reporting each moved
example with the answer it now returns. It MUST distinguish drift from an
unreachable zone with a different exit status, and it SHALL report each example's
measured score beside the recorded one so a score that moves without changing
the ranking is visible without failing the gate.

#### Scenario: A moved answer fails the check

- **WHEN** the zone answers a shipped example with a different glyph or name than the one recorded
- **THEN** the check fails and names that example

#### Scenario: An unreachable zone is not drift

- **WHEN** the zone cannot be reached at all
- **THEN** the check reports that it could not ask, distinctly from an answer that moved

#### Scenario: A score that drifted is reported

- **WHEN** an example's ranking is unchanged but its score differs from the recorded one
- **THEN** the check reports both scores and still passes

### Requirement: Composition examples are discovered from the model

The reachable composition examples SHALL be found by a search over the
vocabulary rather than written by hand: for every pair of entries the search
SHALL report the third entry with the greatest joint similarity to both, and
the same search against an unchanged vocabulary and model MUST report the same
examples with the same scores.

#### Scenario: The search reports a triple with its evidence

- **WHEN** the search reports a reachable example
- **THEN** it carries the two terms, the third entry that won, and that entry's similarity to each term

#### Scenario: A repeated search is comparable

- **WHEN** the search is run again against an unchanged vocabulary and model
- **THEN** it reports the same examples with the same scores

### Requirement: Discovery reports the bounds it searched under

The search SHALL report the dissimilarity ceiling it required of a pair's terms
and the similarity floor it required of the winning entry's legs, so a re-run of
the same vocabulary is comparable with the one it was pinned from. A pair whose
terms are near-duplicates of each other MUST be excluded, because such a pair
resolves to a sibling of the same symbol rather than to a joke.

#### Scenario: The floors are reported

- **WHEN** the search reports its examples
- **THEN** it reports the ceiling and floor it used

#### Scenario: Near-duplicate pairs are excluded

- **WHEN** the two terms of a pair are near-duplicates of each other
- **THEN** the pair is not reported as a composition example

### Requirement: An unreachable joke is shipped as a measured ceiling

When an example's intended answer is not reachable by any ranking the zone can
produce, the example SHALL ship with the answer the zone does return, the
intended answer, and the measurement that shows why — the intended entry's
similarity to each of the query's terms beside the winning entry's. The gallery
MUST state that the intended answer is unreachable rather than omit the example
or present the winner as if it were the intended answer.

#### Scenario: The ceiling is measured, not asserted

- **WHEN** an example's intended answer is unreachable
- **THEN** it records the intended answer, the measured answer, and both similarities

#### Scenario: The gallery admits the gap

- **WHEN** an example ships with an unreachable intended answer
- **THEN** its copy says the intended answer is not reachable and names what is measured instead

### Requirement: A surface offers the shipped examples rather than restating them

The examples a surface offers a reader SHALL come from the shipped set, so
pinning a discovered joke, re-pinning a score, or retiring an example changes
what the surface offers without editing the surface. Each offered example SHALL
carry what the surface needs to name it — at least the query and the mode the
zone answers it in — and a surface MUST NOT offer an example that is not in the
shipped set.

#### Scenario: A pinned joke becomes an offered example

- **WHEN** an example is added to the shipped set and the surface is rebuilt
- **THEN** the surface offers it without any edit to the surface's own content

#### Scenario: A retired example stops being offered

- **WHEN** an example is removed from the shipped set and the surface is rebuilt
- **THEN** the surface no longer offers it

#### Scenario: The example carries its mode

- **WHEN** a surface offers an example
- **THEN** it can name the mode that example demonstrates — a sentence, a glyph, an arithmetic composition, or a conjunction
