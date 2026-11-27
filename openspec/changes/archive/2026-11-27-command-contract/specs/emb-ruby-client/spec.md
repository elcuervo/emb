# Spec Delta

## ADDED Requirements

### Requirement: The gem validates its command surface against the contract

The gem SHALL ship the server's contract and validate its own command surface against it when its specs run: every command name the gem sends SHALL be declared, every keyword token a builder emits SHALL be declared for that command, and the shipped copy SHALL be byte-identical to the repository artifact. The validation SHALL read the contract, not a hand-maintained list.

#### Scenario: An undeclared command fails the build

- **WHEN** the gem sends a command name absent from the shipped contract
- **THEN** the gem's spec suite SHALL fail, naming the command and the call site file

#### Scenario: A dropped keyword fails the build

- **WHEN** a builder sends a command without a keyword the contract declares mandatory for its argv shape (for example `EMB.IMGMULTI` without `OUTPUTS` when a projection was requested)
- **THEN** the gem's spec suite SHALL fail, naming the command and the missing keyword

#### Scenario: A stale vendored copy fails the build

- **WHEN** the vendored contract differs from the repository's `commands.json`
- **THEN** the gem's spec suite SHALL fail and name the sync task that refreshes it

#### Scenario: A matching surface passes

- **WHEN** the gem's command literals and emitted keywords all appear in the shipped contract and the copies match
- **THEN** the validation SHALL pass without listing commands by hand
