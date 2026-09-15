## MODIFIED Requirements

### Requirement: The `emb-top` panel shows real output

The `emb-top` presentation MUST be the dashboard's actual output, replayed from a
recording of a real run, and MUST NOT present invented models, metrics, or rates
as if they were real. It MUST be carried as text — the recorded terminal's own
characters, drawn by the page — rather than as a picture of a terminal, because
the panel exists to be read and the page's own type is sharper at the plate's
measure than any capture of it. It MUST be captioned with the provenance of its
run — when it was captured and which `emb-top` version it shows — and that caption
MUST be produced by the capture rather than typed by hand. Because the captured
version is frozen in the take and its caption, the panel MUST NOT be the element
that carries the site's current `VERSION`: the site's version-bearing elements
remain stamped from `VERSION`, and the panel is labelled as a capture of a past
run. The panel MUST NOT be the site's only quantitative claim.

#### Scenario: The panel is sourced

- **WHEN** the `emb-top` panel is rendered
- **THEN** its rows and figures are those of a recorded run, and a caption beside it states the run's date, the captured `emb-top` version, the model count and the node it was recorded against

#### Scenario: The panel is text, not a picture

- **WHEN** the panel is inspected
- **THEN** it presents the recording's own characters as text — selectable and sharp at any zoom — and no image or video element stands in for the dashboard

#### Scenario: The panel's provenance is not hand-typed

- **WHEN** the plate is regenerated
- **THEN** its caption, its still frame and the take it plays are written by the capture, so no human edits a date, a version or a number into the page

#### Scenario: The panel's version cannot drift

- **WHEN** `VERSION` changes and the site is checked
- **THEN** every element whose version must track the release is stamped from `VERSION` with no hand-typed value remaining, and the panel — whose version is frozen in the take — is captioned as a capture of a past run rather than as the current release

#### Scenario: The panel is not the only number

- **WHEN** the site makes a performance claim
- **THEN** at least one figure on the site is traceable to a `BENCHMARK.md` run with its reproduction command, independently of the captured panel

## ADDED Requirements

### Requirement: The `emb-top` panel's motion is the reader's choice

The panel MAY animate, and where it does it MUST autoplay only when the reader's
motion preference allows it, MUST offer a control that stops and resumes it, and
MUST hold the run's still frame instead of moving wherever motion is unavailable
or unwelcome. The panel MUST NOT present motion that a reader cannot stop, and
the figures it summarises MUST remain available to assistive technology whether
or not it is playing.

#### Scenario: Motion is gated on the reader's preference

- **WHEN** the page is opened with `prefers-reduced-motion: reduce`
- **THEN** the panel does not autoplay and presents the run's still frame

#### Scenario: Motion can be stopped

- **WHEN** the panel is playing
- **THEN** a control pauses it and it holds its frame until resumed

#### Scenario: No scripting

- **WHEN** the page is rendered with scripting unavailable
- **THEN** the panel presents the run's still frame and its caption, complete and unchanged

#### Scenario: The panel is readable by assistive technology

- **WHEN** the panel is read by a screen reader
- **THEN** a sentence stating the run's figures accompanies it, alongside the caption's provenance
