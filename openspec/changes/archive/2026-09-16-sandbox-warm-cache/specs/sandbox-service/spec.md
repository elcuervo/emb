## ADDED Requirements

### Requirement: The sandbox's warm cache survives a restart

The sandbox SHALL persist its cache to the mounted volume and restore it on
startup, so a restart does not discard every cached embedding. The snapshot path
and its automatic-save interval SHALL be declared in the server configuration
the image ships, and the restore and the graceful-stop flush SHALL use the
server's existing defaults rather than a second mechanism. The persisted cache
SHALL stay within the configured byte budget, and the machine's forced-kill
window SHALL be longer than the server's own shutdown budget so a graceful stop
writes the snapshot. A snapshot that is missing, corrupt, or incompatible with
the loaded model set MUST NOT prevent the sandbox from becoming ready.

#### Scenario: The snapshot lives on the volume

- **WHEN** the sandbox's server configuration is inspected
- **THEN** it names a cache snapshot path on the same mounted volume as the models
- **AND** it names an automatic-save interval
- **AND** the server's default startup-restore and shutdown-save behavior is left enabled

#### Scenario: A graceful restart reloads the cache

- **WHEN** the machine is stopped and started again without changing the cache or the models
- **THEN** the server restores the snapshot before it reports ready
- **AND** an entry restored from it answers without re-running inference

#### Scenario: An ungraceful restart is bounded by the interval

- **WHEN** the machine is terminated without running the server's shutdown path
- **THEN** at most the entries inserted since the last automatic save are lost
- **AND** the next boot restores the last complete snapshot

#### Scenario: A bad snapshot starts cold rather than blocking ready

- **WHEN** the snapshot is missing, corrupt, or was written against a different model set
- **THEN** the server starts with an empty or partial cache and still reports ready
- **AND** becoming ready does not depend on the file existing

#### Scenario: The stop signal is not preempted

- **WHEN** the machine is asked to stop
- **THEN** the platform's forced-kill window is longer than the server's own shutdown budget
- **AND** the graceful flush completes rather than being killed mid-write

#### Scenario: A visitor still cannot trigger a write

- **WHEN** a visitor asks the bridge to save state
- **THEN** it is refused with a reason that describes the permitted surface
- **AND** the reason does not claim the sandbox keeps nothing, because its own lifecycle now writes the snapshot
