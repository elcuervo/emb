# Spec Delta

## ADDED Requirements

### Requirement: The fleet view recommends where capacity is needed

The fleet view SHALL render one advisory fleet-level recommendation naming the resource that is binding, the fleet's remaining headroom, and the node to inspect or replace first. It SHALL be derived from the same per-node recommendation signals, and SHALL NOT average one node's binding resource into the fleet's: a node limited by memory in a fleet limited by CPU SHALL be reported as both. It SHALL NOT drive the fleet verdict.

#### Scenario: Different nodes bind on different resources

- **WHEN** most nodes are limited by CPU while one is limited by memory
- **THEN** the fleet recommendation names both, and names the memory-limited node as the one to look at first

#### Scenario: A fallen-behind peer is named first

- **WHEN** one node's tail latency is far above its peers' while its load is comparable
- **THEN** the fleet recommendation names that node ahead of any node-count recommendation

#### Scenario: Uniform headroom recommends nodes

- **WHEN** every node's headroom is small and no node binds on a different resource
- **THEN** the fleet recommendation names adding nodes and states the headroom it is derived from

#### Scenario: A quiet fleet recommends nothing

- **WHEN** no node is near a threshold
- **THEN** the fleet view renders no recommendation and the verdict is unchanged
