# product-site Specification

## Purpose
The `emb` product site (`website/`) is the persistent, self-hosted marketing
surface for the server: a neo-brutalist technical poster that must make a
developer believe the product works before they clone it. This capability covers
the site's content truth rules, its composition contract, and the interactive
proof it offers — including the live console it drives from the sandbox.

## Requirements

### Requirement: README is the source of truth for every claim

Every capability, command, reply shape, number, or output rendered on the site
MUST be traceable to `README.md`, `BENCHMARK.md`, `examples/scripts/`, or the
shipped code. The site MUST NOT introduce a claim the repository does not
support. A claim about an input path, model branch, or distribution channel MUST
be implemented by the shipped scripts or the server; a claim that a shipped
script explicitly documents as unavailable MUST NOT appear.

#### Scenario: A rendered command exists in the reference

- **WHEN** the site displays a Redis command and its reply
- **THEN** that command and that reply shape appear in `README.md`

#### Scenario: A number carries its provenance

- **WHEN** the site displays a measured number
- **THEN** the number appears in `BENCHMARK.md` with its machine and method, and the site either states that provenance or omits the number

#### Scenario: An absent commercial fact stays absent

- **WHEN** the site is reviewed for commercial claims
- **THEN** it contains no customers, logos, testimonials, pricing, hosted-service implication, or service-level commitment

#### Scenario: An input path is implemented before it is advertised

- **WHEN** the site names an input the product accepts
- **THEN** a shipped script or the server implements that input, and any input a shipped script documents as unavailable is either omitted or stated as unavailable

#### Scenario: A version string is generated, never typed

- **WHEN** the site displays a version
- **THEN** that value derives from `VERSION` rather than being transcribed by hand

#### Scenario: Pre-1.0 status is stated

- **WHEN** the site makes a readiness claim
- **THEN** the page states that the project is pre-1.0 and that interfaces may still move, or the readiness claim is removed

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

### Requirement: Three ground-varied capability blocks before the terrain

The site SHALL carry the capability material as three distinct blocks — protocol,
script inference, and operations — placed between the hero and the terrain, and
the terrain SHALL be the last section before the footer. Each block MUST be
given a distinguishable ground or measure so the sequence reads as three
movements rather than one list. The wordmark, claim, six-row feature list, and
pipeline artwork MUST remain unchanged; the hero sub MAY state the product's
release status, and one install line carrying the command, version, licence, and
platforms MAY sit adjacent to the hero actions.

#### Scenario: The capability region covers all four themes

- **WHEN** the capability blocks are rendered
- **THEN** they present the Redis protocol, `EMB.EVAL`/`EMB.EVSHA` script inference, operational commands, and the `emb-top` dashboard

#### Scenario: The poster composition is preserved

- **WHEN** the page is rendered after this change
- **THEN** the masthead, wordmark, claim, six-row feature list and pipeline artwork are pixel-equivalent to their pre-change state

#### Scenario: Copy inside a fixed slot may be corrected

- **WHEN** the hero sub or the install line is changed
- **THEN** the change is confined to copy and to one install element, no composition atom is added or removed, and the claim, wordmark, feature list and pipeline artwork are untouched

#### Scenario: Document height is not a constraint

- **WHEN** the recomposition makes the page longer or changes its ratio
- **THEN** the change is accepted and the ratio is not treated as a regression

#### Scenario: The terrain closes the page

- **WHEN** the page is rendered
- **THEN** the terrain is the last section before the footer and no capability content follows it

#### Scenario: The grounds differ

- **WHEN** the three blocks are compared
- **THEN** at least one is a full-bleed dark ground, and the remaining grounds or measures differ from each other and from the hero

### Requirement: The console runs a live executor

The site SHALL render the realtime-console panel and SHALL drive it with a live
executor that sends an allowlisted command to the sandbox and renders the real
reply. The panel MUST use real form controls (`<form>`, text input, and an
output region with `aria-live="polite"`). The executor MUST be replaceable
behind a documented seam without structural edits to the markup, modes, or
states. The console MUST NOT present an endpoint, host, or hosted-service
affordance, and MUST NOT present a fabricated reply as if it were live. When the
sandbox cannot be reached, the console MUST render its offline state and MUST
NOT fall back to a transcript.

#### Scenario: The panel renders with a live executor

- **WHEN** the shipping revision is opened in a browser
- **THEN** the console panel is visible, its controls are focusable and operable, and submitting a permitted command returns the sandbox's reply

#### Scenario: A reply is the server's, not a transcript

- **WHEN** a permitted command is submitted
- **THEN** the rendered reply is the value returned for that command, and no hand-written transcript line is substituted for it

#### Scenario: The sandbox is unreachable

- **WHEN** the sandbox cannot be reached, is still starting, or is at capacity
- **THEN** the console states that condition and offers a retry, and no reply is fabricated

#### Scenario: The executor is replaceable behind the seam

- **WHEN** a different executor is provided through the documented seam
- **THEN** the same markup, modes, and states drive it without structural edits

#### Scenario: No script, no live panel

- **WHEN** the page is loaded with JavaScript unavailable or failing
- **THEN** the live form is not operable and a static specimen stands in, so the section never presents an empty or broken panel

#### Scenario: The panel is not a hosted-service affordance

- **WHEN** the console is rendered
- **THEN** it describes itself as a sandbox that may reset, and carries no pricing, account, uptime, or support affordance

### Requirement: The console's examples demonstrate the two special functions

The console SHALL demonstrate the product's special functions — an embedding
reply as raw bytes and as a typed envelope, one call answered for several models,
a preloaded script returning a structured non-embedding reply, and the server's
own readouts — without requiring the visitor to select a mode first. Each
demonstration MUST be a command the visitor can submit as it stands, and every
command the sandbox permits MUST be reachable from the demonstrations, so no part
of the demonstrable surface is left to be guessed at. The preset commands MUST be
derived from the digests the server preloaded rather than transcribed, so the
console cannot offer a command the sandbox refuses.

The demonstrations MUST be shown as a ledger beside the console rather than as
part of it: the console is the terminal, and the ledger is what to put in it.
Choosing a demonstration MUST run it in the console, and the ledger MUST NOT be
replaced by a command's output.

#### Scenario: The byte reply and the typed reply are both demonstrable

- **WHEN** the console is idle
- **THEN** it offers an `EMB` call and the same call with `VALUES`, and submitting either shows the reply the sandbox returns for it

#### Scenario: The script reply is demonstrable without Lua source

- **WHEN** the console is idle
- **THEN** it offers a call to a preloaded preset by its model and digest, and submitting it shows a labelled, non-embedding reply

#### Scenario: The digest is the server's

- **WHEN** the preset bytes change under the digest the page carries
- **THEN** the site's own check fails rather than the console offering a digest the sandbox answers `no such script` to

#### Scenario: Protocol mode proves the interface

- **WHEN** the visitor submits the offered `EMB` command
- **THEN** it shows a command any Redis client could issue and the bytes that come back, in both the raw and the `VALUES` form

#### Scenario: Script mode proves extensibility

- **WHEN** the visitor submits the offered preset call
- **THEN** it shows a script the server loaded once being evaluated by digest to produce a labelled reply

#### Scenario: The server's own readouts are demonstrable

- **WHEN** the console is at rest
- **THEN** the commands that describe the server — the loaded models, one model's info, its statistics, its readiness probe, its help, and a ping — are offered alongside the reply forms, and none of them needs a mode or a setting to reach

#### Scenario: The multi-model call is demonstrable

- **WHEN** the console is at rest
- **THEN** it offers an `EMB.MULTI` call naming more than one of the sandbox's models, and submitting it returns one reply carrying a slot for each model

#### Scenario: The reply forms need nothing set first

- **WHEN** the visitor compares the offered `EMB` call with the offered `VALUES` call
- **THEN** the two reply forms are both reachable from the menu, without a protocol version, a format, or any other setting being chosen before either can be read

### Requirement: The console is a REPL with a command menu and recall

The console SHALL present itself as a single prompt and a single transcript
rather than as a set of modes, with the demonstration commands beside the
transcript as a menu the reader can return to. It MUST submit on `Enter`, MUST
offer a recallable history of previously submitted commands through the up and
down arrow keys at the prompt, and MUST treat a command chosen from the menu as
a submitted command for the purpose of that history. Recall MUST stop at both
ends of the history rather than wrapping, and MUST preserve a partially typed
command while the reader walks away from it and back.

A menu row MAY show a shortened label for a command that will not fit its row,
provided the command it submits is the full one.

The console's controls MUST remain operable by keyboard alone with an accessible
name each, and no focusable control MUST be smaller than the site's committed
target floor.

#### Scenario: Enter submits

- **WHEN** the reader types a command and presses Enter
- **THEN** it is submitted, without a separate control being required

#### Scenario: Up and down recall

- **WHEN** the reader presses the up arrow at the prompt
- **THEN** the previous command is placed in the prompt, and the down arrow returns toward the most recent one, stopping at each end

#### Scenario: A chosen command is history

- **WHEN** the reader runs a command from the menu and then presses the up arrow
- **THEN** that command is placed in the prompt

#### Scenario: A draft survives recall

- **WHEN** the reader has typed part of a command, walks back through the history, and then returns to the prompt
- **THEN** the partial command is still there

#### Scenario: A shortened label still submits the whole command

- **WHEN** a menu row's label is shorter than the command it stands for
- **THEN** activating it submits the command in full, and its accessible name contains the label that is drawn

#### Scenario: The panel is operable and named

- **WHEN** the console is navigated by keyboard alone
- **THEN** every control is reachable and operable, each has an accessible name that states what it does, and none is smaller than the committed target floor

#### Scenario: The reply states what it cost

- **WHEN** a command is answered
- **THEN** the reply states how long the server took to answer the call, as measured by the sandbox from the command write through the reply read, and the reader's own network round trip is not shown

#### Scenario: The console is not a mode picker

- **WHEN** the console is rendered
- **THEN** it presents no mode selector and no control whose only purpose is to change what the transcript is about

### Requirement: Console states are complete and honest

The console SHALL define and render an idle state, a running state, a result
state, an error state, a starting state, and an offline state. It MUST NOT
present an empty or broken panel in any of them. The demonstration commands MUST
be part of the panel rather than of a state: they MUST be operable at rest and
MUST still be shown, and still be operable, after a command has been run. The
transcript MUST be bounded, and MUST scroll rather than lengthen the panel, so
that a reply of any size leaves the panel the size the reader found it. The
offline state MUST offer the visitor a next action rather than only reporting
the condition.

#### Scenario: An unknown command fails legibly

- **WHEN** a command outside the permitted set is submitted
- **THEN** the console renders the sandbox's error reply and a hint toward the supported commands

#### Scenario: The idle state teaches by being runnable

- **WHEN** the console is at rest and has not yet run a command
- **THEN** the demonstration commands are shown as operable controls beside it, and one of them is a request for the sandbox's own help

#### Scenario: The console opens on a line, not a void

- **WHEN** the console is at rest with nothing run yet
- **THEN** its transcript carries one line of its own that states how the prompt works, and the first command replaces it

#### Scenario: A command does not take the demonstration away

- **WHEN** a command has been run
- **THEN** the demonstration commands are still shown and still operable, so a second one can be run without reloading the page, and the ledger is untouched by the reply it produced

#### Scenario: A long reply does not grow the panel

- **WHEN** a reply is longer than the transcript's height
- **THEN** the transcript scrolls to its own end and the panel keeps the height it had, and a reader who has scrolled back is not moved while reading

#### Scenario: The sandbox is starting

- **WHEN** a command is submitted while the sandbox reports that it is starting
- **THEN** the console states that it is waking and retries without presenting the condition as a command failure

#### Scenario: The sandbox is offline

- **WHEN** the sandbox is unreachable
- **THEN** the console states that it is offline and offers a retry, the demonstration commands remain operable, and no transcript is fabricated in place of a reply

#### Scenario: Motion preference is honoured

- **WHEN** the reader prefers reduced motion
- **THEN** the reply appears immediately with no typewriter playback

### Requirement: The demo says what answers it and how to drive it

The landing page SHALL state, before the console, what answers a command — a real
`emb` process behind the sandbox bridge, carrying the sandbox's own models — what
the bridge permits and refuses, what a command costs, and how a reader runs one:
a row from the ledger or a typed command, `Enter` to submit, the arrow keys to
recall what has been run, and the round trip printed under each reply. The
explanation MUST be the page's own copy rather than only the console's, and MUST
NOT claim more than the sandbox does: where it is read-only, rate-bounded, or may
reset, the page MUST say so.

The console SHALL be presented on the page's dark ground rather than as a panel
framed in paper, and the ledger beside it MUST invert with that ground, so what
answers is visibly made of the same material as the terminal page itself.

#### Scenario: The page names what answers

- **WHEN** the demo block is read before the console
- **THEN** it states that a real `emb` process answers, and which models the sandbox has loaded

#### Scenario: The page states the refusals

- **WHEN** the reader reads the demo block's facts
- **THEN** the commands the bridge refuses are named as families — configuration, raw Lua, images, writes — rather than left to be discovered by a refusal

#### Scenario: The page states how to drive the panel

- **WHEN** the reader reads the demo block's prose
- **THEN** it states that a ledger row runs a command, that `Enter` submits a typed one, that the arrows recall what has been run, and that each reply is followed by the time it took

#### Scenario: The panel is not framed as an exhibit

- **WHEN** the demo block is rendered at any width
- **THEN** the console and its ledger sit on the block's dark ground under the same hairline rules the rest of that ground uses, and no paper border separates the panel from it

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
- **THEN** the capability region stacks in reading order with no clipped content and no horizontal scroll, and the console panel takes part in that reflow whenever it is enabled

#### Scenario: Semantic structure

- **WHEN** the new region is inspected
- **THEN** its heading levels follow the existing document outline and decorative artwork is hidden from assistive technology

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
run. The panel MUST NOT be the site's only quantitative claim. Where the same
recorded run is presented on more than one surface, every presentation of it
MUST be written from that one run, so the figures cannot disagree between
surfaces.

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

#### Scenario: Two surfaces, one run

- **WHEN** the landing page's plate and the documentation surface's plate are compared
- **THEN** their frames, captions and figures describe the same recorded run, and both are written by the same publishing step rather than maintained by hand

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

### Requirement: The documentation surface carries the `emb-top` plate in the run's own colours, without unsolicited motion

The documentation surface SHALL present the recorded `emb-top` run as the
recording itself — drawn in the dashboard's own colours, with the player's own
transport controls — and MUST NOT begin playing it without the reader asking. It
MUST NOT present the run as plain uncoloured text where the recording can be
drawn: a capture stripped of its ANSI codes is a fallback for a reader the player
cannot reach, not the surface's picture of the dashboard.

The plate MUST hold the run's still frame before any script runs, so that a
reader with scripting unavailable, or a reader the player cannot reach, receives
the same run rather than an empty plate. Where the run is played, the reader MUST
be able to stop it.

#### Scenario: The plate is the recording, in colour

- **WHEN** the documentation surface is rendered with scripting available
- **THEN** the plate shows the recorded run drawn by the player, with the dashboard's own colours, and not the uncoloured text frame that stands in for it

#### Scenario: Motion is requested, not imposed

- **WHEN** the documentation surface is opened with scripting available and the reader has not started the recording
- **THEN** the plate holds a frame of the run, nothing on it is playing, and the player's own control starts it

#### Scenario: Playback can be stopped

- **WHEN** the recording is playing
- **THEN** the player's own transport can pause it and start it again

#### Scenario: No scripting, no empty plate

- **WHEN** the documentation surface is rendered with scripting unavailable or failing, and the tape will not play
- **THEN** the plate shows the run's still frame, its caption and its figures, and the page's content is complete

#### Scenario: A plate too narrow to draw fetches nothing

- **WHEN** the plate is not rendered at the current width, and the run's figures stand in for it
- **THEN** the recording is not fetched and no player is built

#### Scenario: The plate is the same run as the landing's

- **WHEN** the documentation plate and the landing page's plate are compared
- **THEN** they name the same recorded run and present the same figures

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

### Requirement: The spine is one continuous element from the wordmark to the massif

The spine SHALL be a page-level structural element: a single orange line running
from the `emb` wordmark to the terrain, present in every section between them,
with no visible gap or lateral step at any section boundary. Its horizontal
position MUST be derived from one named axis so the hero's SVG spine, the block
spines, and the terrain route cannot drift apart.

#### Scenario: No gap at a section boundary

- **WHEN** the rendered spine is measured at each section boundary
- **THEN** the segments join with no vertical gap and no lateral offset

#### Scenario: One axis, measured

- **WHEN** the spine's rendered x-position is compared with the terrain route's fork position
- **THEN** they agree within 1px at every tested width, including the 1086px reference frame

#### Scenario: The spine never carries text

- **WHEN** body copy is rendered
- **THEN** no text sits on the spine; the line either passes behind content or crosses a ground prepared for it

### Requirement: The spine is drawn from the page's own tokens

The spine MUST use the existing accent and the existing plate geometry; it MUST
NOT introduce a new colour, a new stroke-weight system, or a new asset.

#### Scenario: Spine and plate stroke agree

- **WHEN** the CSS spine's rendered width is compared with the SVG spine's rendered stroke width
- **THEN** they match, including when the plate width is at its minimum

### Requirement: Code is typeset and highlighted as code

Every specimen that is code — shell invocations, Lua source, and the console's
replayed commands and replies — SHALL be presented in the mono face with a token
treatment: command or keyword, string literal, numeric, and comment or meta.
Token colours MUST be defined for both the paper ground and the dark ground, and
every token colour MUST meet WCAG AA contrast against the ground it is used on.

#### Scenario: Tokens are distinguishable and legible

- **WHEN** a code specimen is rendered on paper
- **THEN** each token class has a distinct treatment and every token colour measures at least 4.5:1 against the paper

#### Scenario: The dark ground has its own tokens

- **WHEN** a code specimen is rendered inside the dark block
- **THEN** it uses the dark ground's token colours and every one measures at least 4.5:1 against that ground

#### Scenario: Highlighting is not decoration

- **WHEN** a specimen is examined
- **THEN** the highlighting marks what the token is (a command, a string, a number, a comment) and does not colour text for emphasis

### Requirement: The inverted block inverts the full type ladder

The dark block MUST carry its own ink, dim-voice, rule, and accent values, and
every text and control colour in it MUST meet WCAG AA against its own
background. The page's committed type floor MUST hold inside it.

#### Scenario: The dark ground's floor is measured

- **WHEN** the dark block is rendered at the 1086px reference frame and at 390px
- **THEN** its smallest computed text size is at least 12px and 14px respectively

#### Scenario: The dark ground's contrast is verified

- **WHEN** the dark block's text and control colours are measured
- **THEN** each meets at least 4.5:1 for text and 3:1 for non-text indicators

### Requirement: The terrain's route still starts where the spine ends

The terrain's route MUST begin at the spine's axis, and the spine MUST reach the
terrain's first drawable point, at every width where both are rendered.

#### Scenario: The handover holds

- **WHEN** the terrain is at the end of the page and the reader scrolls to it
- **THEN** the orange line is continuous from the last block into the ridge route, with no gap and no lateral step

### Requirement: The landing carries one install line and one door to the documentation

The landing page SHALL carry exactly one copy-paste install command — stating the
command, the version, the licence, and the supported platforms — placed in the
hero beside the calls to action, in the same block, so a reader who has decided
to try the product can act without leaving the site or the fold's neighbourhood.
Its calls to action and its documentation navigation entries MUST point at the
documentation surface rather than at off-site anchors, and the landing MUST NOT
link the reference material from anywhere else. It MUST NOT carry the full
command tables, the configuration block, or the operational detail, which belong
to the documentation surface.

#### Scenario: A convinced reader can act

- **WHEN** a reader decides to try the product from the landing page
- **THEN** an install command is present in the hero in the same block as the calls to action, and the page's calls to action stay on the site

#### Scenario: The install line is adjacent, not merely present

- **WHEN** the install block is rendered
- **THEN** it sits directly below the hero actions as one ruled row, states the command and the release facts, and is not separated from the calls to action by any other content

#### Scenario: Every documentation link is a door, not a detour

- **WHEN** the landing page's links are reviewed
- **THEN** every call to action and documentation navigation entry resolves to the documentation surface, and no other link on the page points at reference material

#### Scenario: Detail is not duplicated

- **WHEN** the landing page is reviewed for reference material
- **THEN** it contains no command table, no configuration block, and no more than four named commands, and the documentation surface is the only place the full set lives

### Requirement: Text ink stays inside the sheet at every width

Every text element on the site MUST render with its glyph ink inside the
viewport at every supported width. A container may bleed past the sheet edge only
where it carries no text. Where the page frame clips horizontal overflow, that
clip MUST NOT remove any part of a character.

#### Scenario: Ink is measured, not the box

- **WHEN** the page is rendered at any supported width
- **THEN** the rightmost and leftmost ink rect of every text element lies within the viewport, measured on the text runs themselves

#### Scenario: The frame's clip cannot hide a defect

- **WHEN** a horizontal clip is applied to the page frame
- **THEN** the check for overflowing text is performed against text rects, because the clip removes the overflow from the scrollable region and makes a scroll-width comparison report success

#### Scenario: A deliberate bleed carries no text

- **WHEN** a container is positioned past the sheet edge
- **THEN** any text inside it is inset far enough that no glyph reaches the clip boundary

### Requirement: The composed page survives a script failure

The site SHALL render its complete content when its JavaScript fails to load or
throws. Motion and progressive enhancement MAY be gated on a script-provided
class, but no content, artwork, or text MUST be permanently hidden by a class
that only the script can remove.

#### Scenario: The script never arrives

- **WHEN** the site's script fails to load, is blocked, or throws before completing
- **THEN** the hero artwork, the annotations, and every section are visible without interaction

#### Scenario: The enhancement class is reversible

- **WHEN** the script cannot run
- **THEN** the enhancement class is not left applied, so the styles that hide content for animation do not take effect

### Requirement: The printed page keeps its dark surfaces legible

The site's print stylesheet SHALL keep every dark-ground surface readable on
paper: text that is light-on-dark on screen MUST either keep its ground when
printed or be re-coloured for a white ground.

#### Scenario: Dark surfaces print readable

- **WHEN** the page is printed with background graphics disabled
- **THEN** no text is rendered light-on-white, and every dark-ground section remains legible
