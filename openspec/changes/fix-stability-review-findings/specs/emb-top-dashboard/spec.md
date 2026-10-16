# Spec Delta

## ADDED Requirements

### Requirement: emb-top validates its own flag values

`emb-top` SHALL reject a flag value it cannot honor, before it takes the terminal's screen, with a message on stderr and a non-zero exit. A history window SHALL be at least one poll, in every mode of the program, rather than a slice bound that can fall below the ring it trims.

#### Scenario: A window below one is rejected

- **WHEN** user runs `emb-top -window 0`, or `emb-top -window -1`, in the TUI, `-once` or `-frames` mode
- **THEN** `emb-top` SHALL exit non-zero with a message naming `-window`
- **AND** it SHALL NOT panic, and SHALL NOT leave the alternate screen occupied

#### Scenario: Valid values are unaffected

- **WHEN** user runs `emb-top -window 1 -interval 1s -once -samples 2 -addr host:port`
- **THEN** it SHALL sample and print as before
