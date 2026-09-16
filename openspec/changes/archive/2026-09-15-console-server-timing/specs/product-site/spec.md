## MODIFIED Requirements

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
