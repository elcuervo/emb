## MODIFIED Requirements

### Requirement: Lazy pool initialization

Changed from mutex-guarded to `||=`:

#### Scenario: Cold start with concurrent commands

- **WHEN** two threads call `Emb.send_command` simultaneously on a cold start
- **THEN** one pool SHALL be created and used for all commands
- **THEN** no errors SHALL be raised
