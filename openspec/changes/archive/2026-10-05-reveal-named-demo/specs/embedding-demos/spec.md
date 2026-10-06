# Spec Delta

## ADDED Requirements

### Requirement: A fragment that names a choice reveals the demo

When a plate is opened, or its fragment changes, to a name that selects one of
its choices, the plate SHALL bring the demo region into view rather than leaving
the reader at the top of the page. The reveal SHALL run after the choice is
selected, SHALL NOT run when the fragment names no choice, and SHALL scroll
instantly rather than animating. Selecting a choice by clicking its control MUST
NOT scroll the page.

#### Scenario: A naming fragment lands on the demo

- **WHEN** a plate is opened with a fragment that names one of its choices
- **THEN** the page is scrolled so that the demo region is in view, with the sticky masthead not covering its heading

#### Scenario: No naming fragment does not move the page

- **WHEN** a plate is opened with no fragment, or a fragment that names none of its choices
- **THEN** the plate does not scroll the page and renders its default as before

#### Scenario: A changed fragment reveals the choice

- **WHEN** the fragment changes to a name the plate knows while the plate is open
- **THEN** the plate selects that choice and brings the demo region into view

#### Scenario: The reveal is instant

- **WHEN** the reveal runs
- **THEN** the scroll is immediate and does not animate, so it needs no reduced-motion branch

#### Scenario: Selecting by click does not jump

- **WHEN** a reader selects a choice by clicking its control
- **THEN** the plate selects the choice without scrolling the page itself
