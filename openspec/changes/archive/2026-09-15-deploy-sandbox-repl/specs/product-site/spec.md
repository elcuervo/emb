## REMOVED Requirements

### Requirement: The console is a placeholder with a live seam

**Reason**: The live executor the placeholder was waiting for now exists. Keeping
the panel withheld, and keeping a deterministic transcript client behind the
seam, would ship a fabricated reply beside a real one and would keep the site's
strongest proof hidden.

**Migration**: The panel is rendered and driven by the sandbox through the same
seam. The transcript client and the "demo, not a live server" framing are
removed; the offline and starting states take over the failure paths the
withheld state used to cover. See `The console runs a live executor` below.

## ADDED Requirements

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

## MODIFIED Requirements

### Requirement: Console modes demonstrate the two special functions

The console SHALL offer a Redis-protocol mode and a script-inference mode. The
Redis-protocol mode MUST run an `EMB` call and show both the raw float32 reply
and the `VALUES` reply. The script-inference mode MUST call a preset script by
its digest and show its structured, non-embedding reply, without the visitor
supplying Lua source.

#### Scenario: Protocol mode proves the interface

- **WHEN** the Redis-protocol mode is active
- **THEN** it runs a command any Redis client could issue and shows the reply the sandbox returns, in both the raw and the `VALUES` form

#### Scenario: Script mode proves extensibility

- **WHEN** the script-inference mode is active
- **THEN** it calls a preset script by its digest and shows a labeled, non-embedding reply

#### Scenario: The protocol version is observable

- **WHEN** the visitor selects the protocol version
- **THEN** the same command is shown in the flat form and in the typed form as the server returned them, and the difference is visible

### Requirement: Console states are complete and honest

The console SHALL define and render an idle state, a running state, a result
state, an error state, a starting state, and an offline state. It MUST NOT
present an empty or broken panel in any of them.

#### Scenario: An unknown command fails legibly

- **WHEN** a command outside the permitted set is submitted
- **THEN** the console renders the sandbox's error reply and a hint toward the supported commands

#### Scenario: The sandbox is starting

- **WHEN** a command is submitted while the sandbox reports that it is starting
- **THEN** the console states that it is waking and retries without presenting the condition as a command failure

#### Scenario: The sandbox is offline

- **WHEN** the sandbox is unreachable
- **THEN** the console states that it is offline and offers a retry, and does not render a transcript in its place

#### Scenario: Motion preference is honoured

- **WHEN** the reader prefers reduced motion
- **THEN** the reply appears immediately with no typewriter playback
