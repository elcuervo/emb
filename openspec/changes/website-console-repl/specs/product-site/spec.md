## MODIFIED Requirements

### Requirement: The console's examples demonstrate the two special functions

The console SHALL demonstrate both of the product's special functions — an
embedding reply as raw bytes and as a typed envelope, and a preloaded script
returning a structured, non-embedding reply — without requiring the visitor to
select a mode first. Each demonstration MUST be a command the visitor can submit
as it stands, and the commands MUST be derived from the digests the server
preloaded rather than transcribed, so the console cannot offer a command the
sandbox refuses.

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

#### Scenario: The multi-model call is demonstrable

- **WHEN** the console is at rest
- **THEN** it offers an `EMB.MULTI` call naming more than one of the sandbox's models, and submitting it returns one reply carrying a slot for each model

#### Scenario: The reply forms need nothing set first

- **WHEN** the visitor compares the offered `EMB` call with the offered `VALUES` call
- **THEN** the two reply forms are both reachable from the menu, without a protocol version, a format, or any other setting being chosen before either can be read

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
- **THEN** the panel carries the demonstration commands as operable controls, and one of them is a request for the sandbox's own help

#### Scenario: A command does not take the demonstration away

- **WHEN** a command has been run
- **THEN** the demonstration commands are still shown and still operable, so a second one can be run without reloading the page

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

### Requirement: The `emb-top` panel shows real output

Every `emb-top` presentation on the site MUST be derived from the dashboard's
actual output and MUST NOT present invented models, metrics, or rates as if they
were real. Its version string MUST be generated from `VERSION`. Where a
presentation's figures are illustrative rather than drawn from a `BENCHMARK.md`
run, it MUST label them as illustrative and MUST NOT be the site's only
quantitative claim. Where the same recorded run is presented on more than one
surface, every presentation of it MUST be written from that one run, so the
figures cannot disagree between surfaces.

#### Scenario: The panel is sourced

- **WHEN** an `emb-top` presentation is rendered
- **THEN** its rows and figures correspond to a real run or are clearly labelled as illustrative

#### Scenario: The panel's version cannot drift

- **WHEN** `VERSION` changes
- **THEN** the version shown in the panel changes with it and no hand-typed version remains

#### Scenario: The panel is not the only number

- **WHEN** the site makes a performance claim
- **THEN** at least one figure on the site is traceable to a `BENCHMARK.md` run with its reproduction command, independently of the illustrative panel

#### Scenario: Two surfaces, one run

- **WHEN** the landing page's plate and the documentation surface's plate are compared
- **THEN** their frames, captions and figures describe the same recorded run, and both are written by the same publishing step rather than maintained by hand

## RENAMED Requirements

- FROM: `### Requirement: Console modes demonstrate the two special functions`
- TO: `### Requirement: The console's examples demonstrate the two special functions`

## ADDED Requirements

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

#### Scenario: The console is not a mode picker

- **WHEN** the console is rendered
- **THEN** it presents no mode selector and no control whose only purpose is to change what the transcript is about

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
