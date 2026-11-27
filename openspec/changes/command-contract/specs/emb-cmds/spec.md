# Spec Delta

## ADDED Requirements

### Requirement: EMB.HELP lists every declared command

`EMB.HELP` SHALL mention every command the contract declares, so the prose help and the machine-readable contract cannot disagree about which commands exist. Declaring a command without a help line SHALL fail the server's test suite.

#### Scenario: A declared command missing from help fails the build

- **WHEN** a command is declared in the contract but has no line in the `EMB.HELP` body
- **THEN** the server's test suite SHALL fail, naming the missing command

#### Scenario: Help names every declared command

- **WHEN** a client sends `EMB.HELP`
- **THEN** the reply SHALL contain a line for each of `EMB.IMG`, `EMB.IMGMULTI`, `EMB.STATS`, `EMB.SCRIPT`, `CONFIG`, and `SHUTDOWN`
