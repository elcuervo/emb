# Spec Delta

## ADDED Requirements

### Requirement: A demo's in-page selection is named by the URL

A plate that offers a choice among examples or items SHALL name the current
choice in the URL fragment, SHALL restore the choice the fragment names when the
page loads, and SHALL reflect a selection back to the fragment. A fragment the
plate does not know MUST leave its default selection unchanged and MUST NOT be
treated as an error. Reflecting a selection MUST NOT add a history entry, so a
Back gesture leaves the plate rather than walking its selections.

#### Scenario: A shared link opens the named choice

- **WHEN** a plate that offers an in-page choice is opened with a fragment naming one of its choices
- **THEN** that choice is the one selected and rendered

#### Scenario: Selecting a choice names it in the URL

- **WHEN** a reader selects a choice on a plate
- **THEN** the URL fragment names the choice that was selected

#### Scenario: An arriving fragment re-selects on the loaded plate

- **WHEN** the fragment changes to a name the plate knows while the plate is loaded
- **THEN** the plate selects that choice

#### Scenario: An unknown fragment leaves the default

- **WHEN** a plate loads with a fragment naming none of its choices, or with no fragment
- **THEN** it renders its default selection and reports no error

#### Scenario: Selection does not consume the Back gesture

- **WHEN** a reader selects a choice and then navigates Back
- **THEN** the plate is left rather than the previous selection restored
