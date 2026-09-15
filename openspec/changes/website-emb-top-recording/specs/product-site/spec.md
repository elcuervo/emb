## MODIFIED Requirements

### Requirement: The `emb-top` panel shows real output

The `emb-top` presentation MUST be a capture of the dashboard's actual output and
MUST NOT present invented models, metrics, or rates as if they were real. It MUST
be captioned with the provenance of its run — when it was captured and which
`emb-top` version it shows — and that caption MUST be produced by the capture
rather than typed by hand. Because the captured version is frozen in the
recording, the panel MUST NOT be the element that carries the site's current
`VERSION`: the site's version-bearing elements remain stamped from `VERSION`, and
the panel is labelled as a capture of a past run. The panel MUST NOT be the
site's only quantitative claim.

#### Scenario: The panel is sourced

- **WHEN** the `emb-top` panel is rendered
- **THEN** its rows and figures correspond to a recorded run, and a caption beside it states the run's date, the captured `emb-top` version and the node it was recorded against

#### Scenario: The panel's provenance is not hand-typed

- **WHEN** the plate is regenerated
- **THEN** its caption is written by the capture, so no human edits a date or a version into the page

#### Scenario: The panel's version cannot drift

- **WHEN** `VERSION` changes and the site is checked
- **THEN** every element whose version must track the release is stamped from `VERSION` with no hand-typed value remaining, and the panel — whose version is frozen in the recording — is captioned as a capture of a past run rather than as the current release

#### Scenario: The panel is not the only number

- **WHEN** the site makes a performance claim
- **THEN** at least one figure on the site is traceable to a `BENCHMARK.md` run with its reproduction command, independently of the recorded panel

## ADDED Requirements

### Requirement: The `emb-top` plate does not animate without consent

The plate SHALL present a still frame unless the reader's environment allows
motion, and MUST NOT expose the reader to an unconditionally animating element.
The still frame SHALL belong to the same run as the animation, so the plate never
shows two different runs. The dashboard's rows and figures SHALL remain available
to assistive technology as text.

#### Scenario: Reduced motion

- **WHEN** the reader's environment requests reduced motion
- **THEN** the plate shows the still frame and no animation plays

#### Scenario: No scripting

- **WHEN** the page is rendered with scripting unavailable
- **THEN** the plate shows the still frame, with no element of the plate missing or collapsed

#### Scenario: Assistive technology

- **WHEN** the plate is read by a screen reader
- **THEN** a text rendering of the dashboard's rows and figures is available, and the plate is not announced as an image with no content
