# Spec Delta

## Purpose

Publishes emb's command surface as a machine-readable contract that clients can diff against their builds, generated from the server's own definitions and held stable by CI so it cannot drift from the commands the server actually serves.

## ADDED Requirements

### Requirement: The server declares every command once

The server SHALL maintain a single declaration for each command it serves, carrying the command name, a syntax grammar, the keyword tokens it accepts, its arity, and its flags. Route registration and declaration SHALL be the same act, so a command cannot be served without a declaration or declared without being served.

#### Scenario: A new command cannot be served undeclared

- **WHEN** a handler is registered for a command name with no matching declaration
- **THEN** the server's test suite SHALL fail, naming the undeclared command

#### Scenario: A declaration cannot outlive its route

- **WHEN** a declaration exists for a command name that no route serves
- **THEN** the server's test suite SHALL fail, naming the stale declaration

### Requirement: The contract is a checked-in generated artifact

The repository SHALL contain `commands.json`, generated from the server's declarations and carrying a contract format version and one entry per command. A generation command SHALL rewrite the artifact, and a check command SHALL regenerate it in memory and fail on any byte difference.

#### Scenario: Regenerating an unchanged contract is a no-op

- **WHEN** `just commands-gen` runs twice with no declaration changed
- **THEN** the second run SHALL leave `commands.json` byte-identical

#### Scenario: Drift fails the check

- **WHEN** a command's declaration changes and `commands.json` is not regenerated
- **THEN** `just commands-check` SHALL exit non-zero and name the differing command

#### Scenario: CI runs the check

- **WHEN** the repository's CI pipeline runs for a change touching the server
- **THEN** it SHALL run the contract check and fail the build on drift, before any test that needs a model

### Requirement: Each entry describes the argv a client must build

Every entry SHALL carry the command name in its wire spelling, the keyword tokens the command accepts in the order they must appear, and arity: a minimum argument count plus, where the tail repeats, the repetition shape. A command with alternative subcommands SHALL declare them as a keyword choice rather than as separate entries.

#### Scenario: A keyword-bearing command declares its tokens

- **WHEN** a client reads the entry for `EMB.IMG`
- **THEN** the entry SHALL name `BLOB`, `VALUES` and `OUTPUTS` as accepted keywords, in wire order, before the first image argument

#### Scenario: An alternative subcommand is one entry

- **WHEN** a client reads the entries for the script family
- **THEN** it SHALL find one `EMB.SCRIPT` entry whose keywords are `LOAD`, `EXISTS` and `FLUSH`, not three entries

### Requirement: Entries classify commands with flags

Every entry SHALL carry flags drawn from a closed vocabulary that classifies the command for deployments, and no flag SHALL imply behavior the server enforces on its own. The vocabulary SHALL at minimum distinguish commands that mutate server state, commands that expose other clients' traffic, and commands that change configuration.

#### Scenario: Flags identify state mutation

- **WHEN** a deployment reads the flags on `EMB.CACHE.FLUSH` and `EMB.SAVE`
- **THEN** both SHALL carry the state-mutation flag, distinguishing them from read-only commands

### Requirement: Image commands declare their reply shape

The entries for `EMB.IMG` and `EMB.IMGMULTI` SHALL carry a reply schema in JSON Schema draft 2020-12 describing the reply for one image or pair and for several. The remainder of the surface SHALL describe replies in prose, because their shapes are not positional.

#### Scenario: One image is a single keyed map

- **WHEN** a client reads the `EMB.IMG` reply schema for a request carrying one image
- **THEN** the schema SHALL describe a single keyed map of output name to value, not an array of maps

#### Scenario: Several images are an array of maps

- **WHEN** a client reads the `EMB.IMG` reply schema for a request carrying several images
- **THEN** the schema SHALL describe an array with one keyed map per image, and a nullable slot for a failed image
