## MODIFIED Requirements

### Requirement: The console is a placeholder with a live seam

The site SHALL retain a realtime-console panel as a specified artifact and MUST
withhold it from the rendered page, and from the accessibility tree, until a
live executor is provided. While withheld, the panel MUST be present in the
markup, MUST NOT start any client-side behavior, and MUST NOT be reachable by
pointer, keyboard, or assistive technology; the withholding MUST be a property
of the document rather than of a script, so a browser that runs no script still
withholds it. The retained panel MUST use real form controls (`<form>`, text
input, and an output region with `aria-live="polite"`) and, when enabled, MUST
run with no network access, replaying deterministic transcripts. It MUST expose
a documented adapter seam so a real RESP client can replace the transcript
source without restructuring the markup. A single document-attribute change
SHALL restore the placeholder, and providing a live executor through the seam
SHALL drive the same markup, modes, and states without structural edits.

#### Scenario: The panel is withheld from the rendered page

- **WHEN** the shipping revision is opened in a browser
- **THEN** the console panel is not visible and occupies no layout space, none of its controls are focusable, and no transcript client or live region is running

#### Scenario: The withholding is not a script's decision

- **WHEN** the page is loaded with JavaScript unavailable or failing
- **THEN** the console panel is still withheld, because the hide is declared in the markup rather than applied by a script

#### Scenario: The artifact is retained, not deleted

- **WHEN** the source is inspected
- **THEN** the panel's markup, styles, two modes, deterministic transcripts, and the adapter seam are all present, so enabling the live runtime is a wiring change and not a re-authoring

#### Scenario: One attribute restores the placeholder

- **WHEN** the attribute that withholds the panel is removed and the page is loaded
- **THEN** the placeholder panel renders in the page and its controls become operable, exactly as before the panel was withheld

#### Scenario: The console works with no network

- **WHEN** the panel is enabled and a command is submitted with the network unavailable
- **THEN** the console still returns its transcript and reports no error

#### Scenario: The placeholder is not mistaken for a live service

- **WHEN** the panel is enabled
- **THEN** it states that it is a demo transcript and does not present an endpoint, host, or hosted-service affordance

#### Scenario: A real client can replace the transcript

- **WHEN** a live executor is provided through the documented adapter seam
- **THEN** the same markup, modes, and states drive it without structural edits

### Requirement: The capability region is responsive and accessible

The capability region SHALL reflow without horizontal scrolling, SHALL preserve
logical reading order, and SHALL keep every control at or above the site's target
size at mobile widths.

#### Scenario: Mobile reflow

- **WHEN** the page is rendered on a 390px viewport
- **THEN** the capability region stacks in reading order with no clipped content and no horizontal scroll, and the console panel takes part in that reflow whenever it is enabled

#### Scenario: Semantic structure

- **WHEN** the new region is inspected
- **THEN** its heading levels follow the existing document outline and decorative artwork is hidden from assistive technology
