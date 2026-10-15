# Spec Delta

## MODIFIED Requirements

### Requirement: Node specifications accept addresses and DNS names

`emb-top` SHALL accept node specifications through a repeatable `-node` flag and a
comma-separated `-nodes` flag, each entry being `host:port`, `host` (taking the default port),
or a DNS name. A DNS name SHALL be expanded to one node per address record it resolves to.
Addresses that resolve to the same `host:port` SHALL be monitored once. A name that does not
resolve SHALL be retried for a built-in bounded window before the failure is reported.

#### Scenario: A name expands to every address record

- **WHEN** `emb-top -nodes emb.internal` is run and `emb.internal` resolves to three addresses
- **THEN** all three are monitored as separate nodes

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
