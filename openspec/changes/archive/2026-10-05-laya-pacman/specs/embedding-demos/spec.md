# Spec Delta

## ADDED Requirements

### Requirement: A looped demo pauses on a bounded play window and on focus loss

A demo that animates a server-computed trace locally SHALL bound its playback to
a documented active-play window, and SHALL pause — not merely hide — when that
window is spent. It SHALL also pause when the page loses focus or the document
becomes hidden. A paused demo MUST NOT advance frames and MUST NOT request the
next episode, and resuming MUST NOT silently restart or overrun the window.

#### Scenario: The play window is spent

- **WHEN** a looped demo has played for its documented active-play window
- **THEN** it stops advancing frames and reports the pause rather than continuing to animate

#### Scenario: The page loses focus

- **WHEN** the document becomes hidden or the window loses focus while a looped demo is playing
- **THEN** the demo pauses immediately, and no further episode is requested while it is paused

#### Scenario: Resuming continues the window

- **WHEN** the visitor resumes a demo paused by the window rather than by focus
- **THEN** playback continues from where it stopped and the window is not reset

### Requirement: The gallery marks a newly added demo

The gallery SHALL mark a demo that is new in the current release, so a returning
reader can find what changed. The mark SHALL be visible beside the demo's own
name and MUST NOT alter the demo's link or its accessible name.

#### Scenario: A new demo is marked

- **WHEN** a demo is added and the gallery lists it
- **THEN** the demo carries a visible `new` mark, and the marked element still links to and names the demo

#### Scenario: The mark is not permanent

- **WHEN** a later release adds another demo
- **THEN** the mark is removed from the previously new one, so the mark identifies the current addition
