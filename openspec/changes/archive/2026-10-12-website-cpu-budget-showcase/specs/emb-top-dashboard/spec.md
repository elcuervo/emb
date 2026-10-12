## ADDED Requirements

### Requirement: emb-top renders the process CPU plan

`emb-top` SHALL render the process CPU plan it reads from `EMB.STATS`: the node dashboard SHALL show the budget, the admitted tokens in use, the peak, and the detected allowance with its source; the cluster view SHALL show the sum of the reachable nodes' plans; and the headless modes SHALL carry those values per node.

#### Scenario: The node dashboard shows the plan

- **WHEN** a node reports a plan in `EMB.STATS`
- **THEN** its dashboard SHALL show the budget, the tokens in use, the peak, and the allowance with its source, and each model's selected width and share where `EMB.INFO` reports them

#### Scenario: The cluster view sums independent plans

- **WHEN** more than one node is monitored and the nodes report plans
- **THEN** the cluster view SHALL show the summed budget, tokens and allowance, and SHALL state each node's own plan in that node's own section

#### Scenario: The headless modes carry the plan

- **WHEN** `-once` or `-frames` output is consumed programmatically
- **THEN** every node section SHALL carry numeric `cpu_budget`, `cpu_active_tokens`, `cpu_peak_tokens`, `cpu_allowance` and `cpu_budget_enabled` fields

#### Scenario: An absent plan is blank, not disabled

- **WHEN** a node's server does not report the plan fields
- **THEN** no plan text SHALL be rendered and the headless output SHALL omit the plan fields, rather than reporting the budget as disabled
