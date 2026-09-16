## ADDED Requirements

### Requirement: The reply carries the server-measured call time

The bridge SHALL measure the elapsed time of the upstream command on its
loopback connection to `emb` and SHALL carry that value in the reply envelope,
so a client can report the server's answer time without timing its own network.
The measured bracket MUST span the command write through the reply read and
MUST NOT include connection negotiation. A reply the bridge produces without
reaching `emb` MUST carry no elapsed value.

#### Scenario: An answered command carries the server's time

- **WHEN** a permitted command is answered by the server
- **THEN** the reply envelope carries the elapsed time of the loopback round trip, measured by the bridge and not by the client

#### Scenario: A reply with no upstream call carries no time

- **WHEN** the bridge refuses a command or refuses it on a spend bound, so the server is never asked
- **THEN** the reply envelope carries no elapsed time, so the client renders no timing trailer for it

#### Scenario: The client's network is not measured

- **WHEN** the same command is run from two clients at different distances from the sandbox
- **THEN** the reported elapsed time reflects the server's answer time and does not grow with the clients' round trips
