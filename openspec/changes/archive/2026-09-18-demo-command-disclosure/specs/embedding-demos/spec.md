## MODIFIED Requirements

### Requirement: Every demo teaches the same five things in the same order

Each demo SHALL present, in one fixed order: what the reader is looking at, the
interaction itself, what just happened (the path from text through a model to a
vector to a distance), why it matters (the real-world job the demo stands in
for), and the exact commands the demo issued. A demo MUST NOT offer an
interaction without the mechanism that produced it and the commands that ran it,
and the section headings MUST be the same across demos so the gallery reads as
one curriculum rather than a set of unrelated toys.

#### Scenario: A demo carries all five sections

- **WHEN** any demo page is rendered
- **THEN** it presents what it is, the interaction, the mechanism, the real-world use, and the commands, in that order

#### Scenario: The mechanism is named, not implied

- **WHEN** the mechanism section is read
- **THEN** it states that text is tokenized, encoded by a named model, pooled and normalized into a fixed-length vector, and compared by a distance, rather than describing the result as magic

#### Scenario: The commands are the ones that ran

- **WHEN** a demo issues a command to the sandbox
- **THEN** the command shown to the reader is the command that was issued, including the model name, the reply form, and the arguments the demo actually sent

## ADDED Requirements

### Requirement: A demo unfolds the commands its last run issued

Each demo that issues a command to the sandbox SHALL offer, once it has run, a
disclosure that unfolds the exact argv issued in that run: the reader's own text,
the digest the sandbox was given, and every argument in the order sent. The
disclosure MUST reflect the most recent run rather than accumulating earlier
ones, MUST be shown for a run that failed or was refused, and MUST carry nothing
but the commands — no timing, no reply shape, and no query the sandbox never
saw. The fold SHALL open without a script. The demo's static record of its call
MUST remain readable and MUST stay distinguishable from the run's own commands.

#### Scenario: The disclosure carries the run's own arguments

- **WHEN** a reader runs a demo and unfolds the commands
- **THEN** the argv shown carries the text the reader supplied, not a placeholder

#### Scenario: The disclosure is the most recent run's

- **WHEN** a reader runs a demo a second time
- **THEN** the commands shown are the second run's, with no command from the first run still listed

#### Scenario: A refused run still shows what was sent

- **WHEN** a demo's command is refused or fails
- **THEN** the commands issued before the failure are still shown as the commands that ran, and no reply or timing the demo did not receive is shown

#### Scenario: The commands and nothing else

- **WHEN** the disclosure is unfolded
- **THEN** it lists only the argv, with no elapsed time, no reply shape, and no query that ran in the browser

#### Scenario: A digest is the one that was sent

- **WHEN** a scripted demo unfolds its command
- **THEN** the digest shown is the one the sandbox ran, while the demo's static record keeps its placeholder shape

#### Scenario: A binary argument is named, not dumped

- **WHEN** a demo sends a binary argument, as the image preset does
- **THEN** the disclosure marks that argument as bytes of a stated length rather than the encoded payload

#### Scenario: Long text is clamped, not laid out in full

- **WHEN** a surface the page did not author carries more text than it can show — the run's own command, or a passage a plate quotes
- **THEN** that text is clamped to a few lines and marked as truncated, and the full text remains in the document rather than being cut from it

#### Scenario: The fold opens on the site's motion, or not at all

- **WHEN** a reader opens the commands
- **THEN** the disclosure unfolds on the site's own easing, and with reduced motion asked for the commands are shown in one frame without the unfold

#### Scenario: The fold needs no script

- **WHEN** a demo page is loaded with scripting disabled
- **THEN** the demo's static record of its call is still readable and the disclosure is absent rather than shown as an error

### Requirement: A demo offers the next plate at its top and its foot

A demo SHALL link to the next plate in the reading order from the top of the
page as well as from the rail at its foot, so a reader who has finished the
section in view can continue without scrolling the whole plate. The link MUST
name the plate it leads to and MUST be built from the page's existing type and
rule vocabulary. The last plate in the order, which has no next, is not required
to carry one.

#### Scenario: The next plate is reachable from the top

- **WHEN** a reader has finished a section near the top of a demo
- **THEN** the next plate in the reading order is linked from the top of the page, and the link names it

#### Scenario: The top link agrees with the foot rail

- **WHEN** a demo carries both a top link and a foot rail
- **THEN** both point at the same plate

#### Scenario: The last plate is honest about having no next

- **WHEN** the final plate in the order is read
- **THEN** it carries no top link to a plate that does not follow it
