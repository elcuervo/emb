# product-site Specification

## Purpose
The `emb` product site (`website/`) is the persistent, self-hosted marketing
surface for the server: a neo-brutalist technical poster that must make a
developer believe the product works before they clone it. This capability covers
the site's content truth rules, its composition contract, and the interactive
proof it offers — including the console placeholder.

## Requirements

### Requirement: README is the source of truth for every claim

Every capability, command, reply shape, number, or output rendered on the site
MUST be traceable to `README.md`, `BENCHMARK.md`, `examples/scripts/`, or the
shipped code. The site MUST NOT introduce a claim the repository does not
support.

#### Scenario: A rendered command exists in the reference

- **WHEN** the site displays a Redis command and its reply
- **THEN** that command and that reply shape appear in `README.md`

#### Scenario: A number carries its provenance

- **WHEN** the site displays a measured number
- **THEN** the number appears in `BENCHMARK.md` with its machine and method, and the site either states that provenance or omits the number

#### Scenario: An absent commercial fact stays absent

- **WHEN** the site is reviewed for commercial claims
- **THEN** it contains no customers, logos, testimonials, pricing, hosted-service implication, or service-level commitment

### Requirement: New sections inherit the poster's composition vocabulary

Any section added to the site MUST be composed from the existing design atoms —
`rule-head` with its hairline rule, ruled numbered entries, `feat` rows, isometric
plates, the orange signal axis, and the annotation form. Sections MUST NOT
introduce cards, gradients, rounded corners, drop shadows beyond the button's
offset, glass effects, or a second accent colour.

#### Scenario: A new block is built from existing atoms

- **WHEN** a new section ships
- **THEN** every visual primitive it uses already exists in `assets/css/styles.css` or is derived from an existing token without adding a new visual language

#### Scenario: A banned device is refused

- **WHEN** a proposed section uses a card grid, gradient, rounded panel, or decorative shadow
- **THEN** it is rejected and rebuilt from the poster's atoms

### Requirement: One merged capability block below the terrain

The site SHALL carry a single merged capability region, placed after the terrain
section, that covers the protocol-as-SDK, script inference, ops-readiness, and
`emb-top`. The hero, wordmark, claim, sub, actions, six-row feature list, pipeline
artwork, terrain, and footer MUST remain unchanged.

#### Scenario: The capability region covers all four themes

- **WHEN** the capability region is rendered
- **THEN** it presents the Redis protocol, `EMB.EVAL`/`EMB.EVSHA` script inference, operational commands, and the `emb-top` dashboard

#### Scenario: The poster composition is preserved

- **WHEN** the page is rendered after this change
- **THEN** the masthead, hero spread, pipeline, and terrain are pixel-equivalent to their pre-change state

#### Scenario: Document height is not a constraint

- **WHEN** the new region makes the page taller than the poster's 1:1.33 ratio
- **THEN** the change is accepted and the ratio is not treated as a regression

### Requirement: The console is a placeholder with a live seam

The site SHALL render a realtime-console panel that is explicitly a placeholder:
it MUST use real form controls (`<form>`, text input, and an output region with
`aria-live="polite"`) and MUST run with no network access, replaying deterministic
transcripts. It MUST expose a documented adapter seam so a real RESP client can
replace the transcript source without restructuring the markup.

#### Scenario: The console works with no network

- **WHEN** the page is loaded and a command is submitted with the network unavailable
- **THEN** the console still returns its transcript and reports no error

#### Scenario: The placeholder is not mistaken for a live service

- **WHEN** the console is rendered
- **THEN** it states that it is a demo transcript and does not present an endpoint, host, or hosted-service affordance

#### Scenario: A real client can replace the transcript

- **WHEN** a live executor is provided through the documented adapter seam
- **THEN** the same markup, modes, and states drive it without structural edits

### Requirement: Console modes demonstrate the two special functions

The console SHALL offer a Redis-protocol mode and a script-inference mode. The
Redis-protocol mode MUST show an `EMB` call and its float32 reply, and MUST show
the RESP3 `VALUES` reply. The script-inference mode MUST show loading a script by
SHA and evaluating it to a structured, non-embedding reply.

#### Scenario: Protocol mode proves the interface

- **WHEN** the Redis-protocol mode is active
- **THEN** it shows a command any Redis client could issue and the bytes that come back

#### Scenario: Script mode proves extensibility

- **WHEN** the script-inference mode is active
- **THEN** it shows a script loaded once and evaluated by SHA to produce a labeled reply

### Requirement: Console states are complete and honest

The console SHALL define and render an idle state, a running state, a result
state, an error state, and an offline or placeholder state. It MUST NOT present
an empty or broken panel in any of them.

#### Scenario: An unknown command fails legibly

- **WHEN** a command outside the demonstrated set is submitted
- **THEN** the console renders a Redis-style error reply and a hint toward the supported commands

#### Scenario: Motion preference is honoured

- **WHEN** the reader prefers reduced motion
- **THEN** the transcript appears immediately with no typewriter playback

### Requirement: The console is usable by keyboard and screen reader

The console's controls MUST be reachable and operable by keyboard alone, MUST
have an accessible name, MUST announce results through a live region, and MUST
have no focusable element smaller than the site's committed target floor.

#### Scenario: Keyboard-only operation

- **WHEN** a reader tabs to the console
- **THEN** they can select a mode, enter a command, submit it, and hear or read the result

#### Scenario: Results are announced

- **WHEN** a command completes
- **THEN** the output region announces the new result to assistive technology

### Requirement: The dark console surface meets its own type and contrast floor

The console is a light-on-dark surface. Its smallest text MUST meet the site's
committed 12px desktop / 14px mobile floor, and every text and control colour on
it MUST meet WCAG AA contrast against its own background.

#### Scenario: The floor is measured, not assumed

- **WHEN** the console is rendered at the 1086px reference frame and on a 390px viewport
- **THEN** its smallest computed text size is at least 12px and 14px respectively

#### Scenario: Dark-surface contrast is verified

- **WHEN** the console's text and control colours are measured
- **THEN** each meets at least 4.5:1 for text and 3:1 for non-text indicators

### Requirement: The capability region is responsive and accessible

The capability region SHALL reflow without horizontal scrolling, SHALL preserve
logical reading order, and SHALL keep every control at or above the site's target
size at mobile widths.

#### Scenario: Mobile reflow

- **WHEN** the page is rendered on a 390px viewport
- **THEN** the console and the merged block stack in reading order with no clipped content and no horizontal scroll

#### Scenario: Semantic structure

- **WHEN** the new region is inspected
- **THEN** its heading levels follow the existing document outline and decorative artwork is hidden from assistive technology

### Requirement: The `emb-top` panel shows real output

The `emb-top` presentation MUST be derived from the dashboard's actual output and
MUST NOT present invented models, metrics, or rates as if they were real.

#### Scenario: The panel is sourced

- **WHEN** the `emb-top` panel is rendered
- **THEN** its rows and figures correspond to a real run or are clearly labelled as illustrative

### Requirement: Impeccable governs the design work

The change MUST be executed through the Impeccable workflow: `impeccable context`
run once for the target, a surface brief carrying a direction contract written
before any markup edit, the detector run once over the changed files after the
build, and `DESIGN.md` updated only when the user approves a durable system
change. The established visual world MUST be inherited rather than replaced.

#### Scenario: Brief precedes build

- **WHEN** the first markup edit is made
- **THEN** a surface brief with the direction contract already exists for the target

#### Scenario: Detection runs once, after the build

- **WHEN** the new section is complete
- **THEN** `impeccable detect --json` has been run once over the changed HTML and CSS files and its findings are resolved or recorded

#### Scenario: The world is not replaced

- **WHEN** the change is reviewed
- **THEN** the palette, type scale, and component language match `DESIGN.md`, and any durable system change to them was explicitly approved by the user

### Requirement: Excluded devices

The site MUST NOT introduce device families the design brief excludes. In
addition to the composition ban above, it MUST NOT add an AI-startup register:
purple gradients, glowing spheres, particle fields, feature card grids, rounded
SaaS panels, testimonial sections, pricing tables, customer logos, animated
blobs, or glassmorphism.

#### Scenario: Review against exclusions

- **WHEN** the new region is reviewed before merge
- **THEN** none of the excluded device families are present in the markup or stylesheet
