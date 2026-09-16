# sandbox-live-stats Specification

## Purpose

A live, read-only view of an `emb-top` frame stream at the sandbox origin, so a
visitor can watch the dashboard of a real `emb` node answering real traffic
without running the tool or being able to drive it.

## Requirements

### Requirement: The sandbox serves a live, read-only dashboard view

The sandbox SHALL serve a live view of the dashboard at a fixed path that
presents the frame stream and nothing else. The view and its stream MUST be
read-only: the stream endpoint MUST accept no command, parameter, or body that
affects the node or the producer, the page MUST carry no control that sends
input anywhere, and no path from the page may reach the server's command surface.
The page MUST state what it is — a live dashboard of the sandbox's own node —
rather than presenting itself as a general monitoring service.

#### Scenario: A viewer watches the live dashboard

- **WHEN** a visitor opens the view
- **THEN** the dashboard's current frame is shown and advances on the producer's poll interval without any interaction

#### Scenario: The view cannot be driven

- **WHEN** a visitor inspects the page and its requests
- **THEN** there is no control, form, or parameter that submits work, and the stream endpoint refuses or ignores any input and only ever emits frames

#### Scenario: The view is honest about itself

- **WHEN** the view is rendered
- **THEN** it names the sandbox's node as the subject and does not offer to monitor another host

### Requirement: One producer serves every viewer

The sandbox SHALL run a single frame producer that polls the node and fans each
frame out to every connected viewer, so the node's polling cost MUST NOT grow
with the number of viewers. The sandbox SHALL bound how many viewers subscribe
at once and SHALL answer beyond the bound with a legible state rather than
unbounded subscription.

#### Scenario: Viewers share one stream

- **WHEN** several viewers open the view at once
- **THEN** the node is polled once per interval, not once per viewer, and every viewer receives the same frames

#### Scenario: Subscriptions are bounded

- **WHEN** more viewers subscribe than the sandbox admits
- **THEN** further subscriptions are refused legibly and the existing viewers are not disrupted

### Requirement: The view shows the dashboard's own output

The view SHALL present the dashboard's own rendered frames — the same
information and the same color as the `emb-top` terminal — rather than a second,
independent rendering of the node's metrics. Each displayed frame MUST be the
producer's complete frame, so what a viewer sees is what the tool would paint.
The view SHALL keep a bounded recent history consistent with the dashboard's own
window, no shorter than five minutes at the default poll interval.

#### Scenario: The frames are the dashboard's

- **WHEN** the live view and a terminal running `emb-top` against the same node are compared
- **THEN** they show the same panels and the same values, derived from the same rendering rather than a parallel implementation

#### Scenario: Color is preserved

- **WHEN** a frame is displayed
- **THEN** the dashboard's colors survive to the page instead of being stripped by the producer's non-terminal output

#### Scenario: The window is bounded and at least five minutes

- **WHEN** the view has been live for longer than the history window
- **THEN** the oldest history is dropped and the view still shows at least the most recent five minutes at the default poll interval

### Requirement: A late viewer sees the current dashboard

A viewer that joins an already-running stream SHALL be shown the dashboard's
current frame, not a partial repaint or an empty terminal, so the view is
correct at any moment rather than only if opened before the producer started.

#### Scenario: Joining after the stream started

- **WHEN** a viewer opens the view long after the producer began
- **THEN** the first frame it displays is the dashboard's current complete frame

#### Scenario: Only complete frames are displayed

- **WHEN** a frame is delivered to a viewer
- **THEN** it is a whole frame, so no viewer ever renders a half-painted dashboard

### Requirement: The standalone terminal links to the live view

The sandbox's standalone terminal SHALL link to the live view, so the view is
reachable from the sandbox's own surface without knowing its path.

#### Scenario: The door is on the terminal

- **WHEN** the standalone terminal is opened
- **THEN** it offers a link to the live view and following that link shows the live dashboard

### Requirement: The live view fails legibly

When the producer or the node it polls is unavailable, the view SHALL show an
honest state that names the condition and continues to try, rather than a stale
frame presented as live or an empty page.

#### Scenario: The node is unreachable

- **WHEN** the producer cannot reach the node
- **THEN** the view reports that the dashboard is unavailable and does not present the last frame as current

#### Scenario: Recovery is automatic

- **WHEN** the node returns
- **THEN** the view resumes advancing on its own without the viewer reloading
