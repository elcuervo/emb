# Spec Delta

## ADDED Requirements

### Requirement: The decision checkpoint uses the machine's cores

The sandbox's shipped server configuration SHALL let the real decision
checkpoint's inference use more than one CPU when the machine has more than one,
and MUST NOT cap the server process to a single thread. The checkpoint's
auxiliary parallelism (inter-op) SHALL stay at one, because its graph is a
chain, and the variation comes only from intra-op threads.

#### Scenario: The shipped configuration is sized to the machine

- **WHEN** the sandbox's server configuration and image environment are inspected
- **THEN** the real checkpoint's `intra_op_threads` is either unset (so the
  documented `cores−2` default applies) or an explicit value greater than one on
  the multi-core machine
- **AND** no process-wide single-thread cap (such as `OMP_NUM_THREADS=1`) is set

#### Scenario: A cold call is not single-threaded

- **WHEN** the sandbox has no warm cache and the plate's typed-question call runs
  on the multi-core machine
- **THEN** its measured inference time reflects more than one intra-op thread,
  materially below the same call's single-thread baseline

### Requirement: The decision plate's fixed payloads answer from a warm cache

The sandbox SHALL declare the typed-question plate's fixed payloads — the Inbox
tickets and the Quickstart round — against the preloaded preset, so the server
warms their replies after the checkpoint loads. A declared payload's first
visitor call after the warm SHALL be served from the reply cache rather than
re-running inference. The warm SHALL be an optimization, not an allowlist: a
payload the sandbox did not declare SHALL still be answered, cold.

#### Scenario: The first visitor gets a cached reply

- **WHEN** the sandbox has finished booting and a visitor runs a declared
  payload for the first time
- **THEN** the call is served from the reply cache, so its server-measured time
  is the cache's, far below the cold inference cost

#### Scenario: Readiness does not wait for the warm

- **WHEN** the warm is still running
- **THEN** the server is already reported ready and answers other commands

#### Scenario: An undeclared payload still answers

- **WHEN** a visitor runs a state the sandbox did not declare
- **THEN** the call runs inference and answers normally, so the warm only removes
  cold starts and never narrows what can be asked
