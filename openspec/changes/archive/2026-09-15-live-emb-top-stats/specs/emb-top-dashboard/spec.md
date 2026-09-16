## ADDED Requirements

### Requirement: emb-top streams complete dashboard frames headlessly

`emb-top` SHALL support a headless streaming mode that renders the dashboard's
own frames to stdout on the poll interval, without a TTY, without the alternate
screen, and without reading input. Each emitted frame MUST be the dashboard's
complete current frame — the same content the TUI paints — so a consumer needs
no terminal emulation and can display only the most recent frame and still be
correct. The mode MUST preserve the dashboard's colors when stdout is not a
terminal rather than letting the output profile strip them, and it SHALL keep
polling and resume after a connection loss instead of exiting.

#### Scenario: Frames stream on the poll interval

- **WHEN** the mode is run against a running node
- **THEN** it prints one complete frame per poll interval until it is stopped

#### Scenario: No terminal is required

- **WHEN** the mode's output is a pipe rather than a terminal
- **THEN** it still emits the dashboard's frames, with color, and opens no pty or alternate screen

#### Scenario: A frame is complete

- **WHEN** a consumer discards every frame but the latest
- **THEN** the latest frame alone renders the dashboard's current state, because each frame is self-contained

#### Scenario: The node restarts under the stream

- **WHEN** the node stops and returns while the mode is running
- **THEN** the mode keeps running, reports the connection loss in its frames, and resumes rendering with counters rebased
