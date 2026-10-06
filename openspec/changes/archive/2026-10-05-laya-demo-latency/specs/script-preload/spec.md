# Spec Delta

## ADDED Requirements

### Requirement: Boot-time warm payloads

A preloaded script entry SHALL be able to declare warm payloads — the runtime
arguments and texts a client would send — and the server SHALL evaluate each
once after the model and script load, through the ordinary scripted path, so the
reply is stored under exactly the key a client call would use. Declaring none
SHALL leave boot unchanged.

#### Scenario: A declared payload is cached at boot

- **WHEN** a model entry preloads a script with a declared warm payload and the
  server reports ready
- **THEN** a client call with the same model, script, texts, and arguments is
  served from the reply cache without running inference

#### Scenario: No declarations, no change

- **WHEN** no script entry declares warm payloads
- **THEN** boot behaviour, readiness, and the cache are unchanged

### Requirement: The warm does not gate readiness or startup

The warm MUST NOT gate readiness: the server SHALL report ready while it runs.
A warm payload that fails MUST be logged and MUST NOT prevent startup or the
remaining payloads.

#### Scenario: Readiness during the warm

- **WHEN** the warm is still running
- **THEN** the server is reported ready and answers other commands

#### Scenario: A failing warm payload does not stop boot

- **WHEN** a declared payload errors (for example its arguments no longer match
  the script's contract)
- **THEN** the failure is logged, the server still starts and reports ready, and
  the other declared payloads still run

### Requirement: The warm yields to client traffic

Because a warm payload occupies the same inference capacity a visitor's call
needs, the warm SHALL stop when a client request has been served since the
previous payload, rather than making a visitor wait behind the whole declared
set. Payloads skipped this way SHALL remain executable, cold, exactly as if they
had never been declared.

#### Scenario: A visitor is not queued behind the warm

- **WHEN** a client request arrives while the warm is between payloads
- **THEN** the warm stops and the client's request is not delayed by the
  remaining payloads

#### Scenario: A skipped payload still answers

- **WHEN** the warm stopped early and a client later runs a payload it had not
  reached
- **THEN** the call runs inference and answers normally
