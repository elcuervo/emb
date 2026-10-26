# Spec Delta

## MODIFIED Requirements

### Requirement: Node specifications accept addresses and DNS names

`emb-top` SHALL accept node specifications through a repeatable `-node` flag and a
comma-separated `-nodes` flag, each entry being `host`, `host:port`, or a DNS name.
The host alone decides which: a non-IP host other than the loopback alias is a name
whose address records are joined with the entry's port, expanded to one node per record.
Addresses that resolve to the same `host:port` SHALL be monitored once. A name that does not
resolve SHALL be retried for a built-in bounded window before the failure is reported.
An IP literal and the loopback alias (`localhost`, in any case and with or without a trailing
dot) SHALL be a single literal address.

#### Scenario: A name expands to every address record

- **WHEN** `emb-top -nodes emb.internal` is run and `emb.internal` resolves to three addresses
- **THEN** all three are monitored as separate nodes

#### Scenario: A name with a port expands to every address record on that port

- **WHEN** `emb-top -nodes emb.internal:6399` is run and `emb.internal` resolves to three addresses
- **THEN** all three are monitored as separate nodes, each dialed at its address on port `6399` and labelled and keyed by that address, and none of them is dialed on the default port

#### Scenario: The loopback alias is one literal address

- **WHEN** `-nodes localhost:6379` is run and the machine answers for both `127.0.0.1` and `::1`
- **THEN** one node is monitored as the specification wrote it, not one node per loopback record

#### Scenario: A port-carrying name that never resolves fails the start

- **WHEN** `-nodes emb.internal:6399` names no address for the whole wait window
- **THEN** `emb-top` reports the resolver's error naming the specification instead of monitoring the un-resolved text as an unreachable row

#### Scenario: Addresses and names mix in one fleet

- **WHEN** `-node db-a:16379 -nodes emb.internal` is run
- **THEN** the named node and every expanded address are monitored in one fleet

#### Scenario: Duplicate resolution is monitored once

- **WHEN** two specifications resolve to the same `host:port`
- **THEN** that node appears once in the fleet and is polled on one connection

#### Scenario: A name whose records arrive while the fleet starts is monitored

- **WHEN** `emb-top` starts against a DNS name that does not resolve yet and its records appear within the wait window
- **THEN** the nodes at those addresses are polled and rendered, and no error is reported

#### Scenario: A specification that resolves to nothing is an error

- **WHEN** a specification names no address at all for the whole wait window
- **THEN** `emb-top` reports a clear error naming the specification, the name as it was queried and the resolver that answered, and does not start presenting an empty fleet as healthy
