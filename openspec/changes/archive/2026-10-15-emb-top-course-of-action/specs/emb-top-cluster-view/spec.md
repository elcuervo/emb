# Spec Delta

## MODIFIED Requirements

### Requirement: The fleet view recommends where capacity is needed

The fleet view SHALL render one advisory fleet-level recommendation naming the resource that is binding, the action that changes it, the fleet's remaining headroom, and the node to inspect or replace first. It SHALL be derived from the same per-node recommendation signals, and SHALL NOT average one node's binding resource into the fleet's: a node limited by memory in a fleet limited by CPU SHALL be reported as both. It SHALL NOT drive the fleet verdict.

When the binding resource is capacity, the recommendation SHALL state the action that adds it (more CPU to a node, more nodes to the fleet); when it is not — a node whose peers are serving the same traffic from the cache, a node whose own cache would remove the work, or a node whose tail is one cold run — the recommendation SHALL name that action instead and SHALL NOT tell the operator to buy capacity. The node it names first SHALL carry that node's attributed tail, so the fleet line points at a phase rather than at a hostname. The recommendation's magnitude floors SHALL be the node's: a fleet claim SHALL NOT fire on evidence that would be too thin to fire for a single node.

#### Scenario: Different nodes bind on different resources

- **WHEN** one node is limited by memory and another by CPU
- **THEN** the fleet recommendation names both resources rather than averaging them

#### Scenario: A fallen-behind peer is named first

- **WHEN** one node's tail latency is far above its peers' while it carries comparable load
- **THEN** that node is named as the one to inspect first, with the phase its tail was attributed to

#### Scenario: Uniform headroom recommends nodes

- **WHEN** every node is near its CPU budget
- **THEN** the fleet recommendation names adding nodes as the action, with the fleet's remaining headroom

#### Scenario: A quiet fleet recommends nothing

- **WHEN** no node binds on a resource and no node's tail stands out
- **THEN** no fleet recommendation is rendered

#### Scenario: A reservable fix outranks capacity

- **WHEN** a node's tail is dominated by its own cache's misses while the fleet has CPU headroom
- **THEN** the fleet recommendation names the cache as the action rather than adding capacity

#### Scenario: A thin fleet tail is not a capacity claim

- **WHEN** a node's tail rests on too few events to attribute
- **THEN** the fleet recommendation SHALL NOT name that node's capacity as the fix
