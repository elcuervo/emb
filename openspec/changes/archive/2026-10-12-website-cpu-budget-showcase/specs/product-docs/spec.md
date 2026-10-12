## MODIFIED Requirements

### Requirement: The docs surface carries the adaptive-capacity capability

The docs surface SHALL document the runtime capacity controller and the process CPU budget inside its existing section order: the `capacity`, `autotune`, `cpu_budget`, `cpu_reserve`, `calibrate`, and model `weight` configuration keys in the configuration section, the autotune observability fields, the process CPU plan fields, the plan's kill switch (`cpu_budget: off`) and the calibration fallback reasons in the operations section, and an adaptive-capacity benchmark subsection in the benchmarks section. No new top-level section SHALL be introduced.

#### Scenario: The configuration keys are documented

- **WHEN** a reader opens the configuration section
- **THEN** it states `capacity` (`auto`, `latency`, `throughput`) and `autotune` (`auto`, `callers`, `off`), their defaults, and the creation-time layout each profile pins

#### Scenario: The observable state is documented

- **WHEN** a reader opens the operations section
- **THEN** it states `script_traffic_class`, `script_inflight`, `script_concurrency_current`, `script_concurrency_target`, and `script_autotune_active`, and names `autotune: off` as the switch that fixes concurrency

#### Scenario: The benchmark subsection exists

- **WHEN** a reader opens the benchmarks section
- **THEN** it carries an adaptive-capacity subsection with the measured figures and the command that reproduces them

#### Scenario: The process budget keys are documented

- **WHEN** a reader opens the configuration section
- **THEN** it states `cpu_budget` (`auto`, `off`, or a positive integer), `cpu_reserve` and `calibrate` with their defaults and precedence, that an explicit `workers` or `intra_op_threads` is never replaced, and that `cpu_budget: off` restores the legacy width rule

#### Scenario: The selected layout and the fallback are documented

- **WHEN** a reader opens the operations section
- **THEN** it states the plan fields the server reports (`cpu_allowance`, `cpu_allowance_source`, `cpu_budget`, `cpu_active_tokens`, `cpu_peak_tokens`, and each model's selected width), the startup cost of the trials, and the reasons a calibrated layout falls back

## ADDED Requirements

### Requirement: The process-budget claim is scoped to what was measured

The docs surface MUST name the policy that produced each benchmark figure (`cpu_budget: auto` or `cpu_budget: off`), MUST NOT attribute the capacity controller's gains to the process budget or the reverse, and SHALL state that the budget bounds admitted intra-op work rather than physical CPU.

#### Scenario: The policy behind a figure is named

- **WHEN** the docs present a throughput or latency figure for the default configuration
- **THEN** they state that the run used `cpu_budget: auto` with calibration enabled, and which width and session count it selected

#### Scenario: Attributions are not mixed

- **WHEN** the docs state what the process budget improved
- **THEN** they name its own pre-change baseline and do not present the capacity controller's out-of-the-box gains as its effect

#### Scenario: The bound is stated honestly

- **WHEN** the docs describe what the budget limits
- **THEN** they state that it bounds admitted intra-op work, not physical CPU, that tokenization and Go work sit outside it, and that shared embedding and image sessions do not spin under the automatic policy
