# Spec Delta

## ADDED Requirements

### Requirement: The docs surface carries the adaptive-capacity capability

The docs surface SHALL document the runtime capacity controller inside its
existing section order: the `capacity` and `autotune` configuration keys in the
configuration section, the autotune observability fields and the kill switch in
the operations section, and an adaptive-capacity benchmark subsection in the
benchmarks section. No new top-level section SHALL be introduced.

#### Scenario: The configuration keys are documented

- **WHEN** a reader opens the configuration section
- **THEN** it states `capacity` (`auto`, `latency`, `throughput`) and `autotune` (`auto`, `callers`, `off`), their defaults, and the creation-time layout each profile pins

#### Scenario: The observable state is documented

- **WHEN** a reader opens the operations section
- **THEN** it states `script_traffic_class`, `script_inflight`, `script_concurrency_current`, `script_concurrency_target`, and `script_autotune_active`, and names `autotune: off` as the switch that fixes concurrency

#### Scenario: The benchmark subsection exists

- **WHEN** a reader opens the benchmarks section
- **THEN** it carries an adaptive-capacity subsection with the measured figures and the command that reproduces them

### Requirement: Adaptive-capacity figures derive from recorded measurements

Every figure on the docs surface that presents adaptive-capacity or autotune
behaviour SHALL be drawn from a measurement recorded in `BENCHMARK.md`, and
SHALL be captioned with the figures it draws. The figures SHALL be composed
from existing design tokens with at most one accent signal.

#### Scenario: A figure's numbers have provenance

- **WHEN** a figure presents a throughput, latency, allowance, or in-flight value
- **THEN** that value appears in `BENCHMARK.md` with its machine, corpus, and reproduction command

#### Scenario: The caption carries the real figures

- **WHEN** a figure is reviewed
- **THEN** its caption states the values the figure draws rather than a qualitative summary

#### Scenario: The figures add no new visual language

- **WHEN** a figure is rendered
- **THEN** it uses the poster's existing atoms — ruled graticule, mono caps labels, small filled marks, one accent — and introduces no second hue, gradient, radius, or shadow

### Requirement: The adaptive-capacity claim is scoped to what was measured

The docs surface MUST attribute the out-of-the-box improvement to the
adaptive-capacity work as a whole, MUST NOT credit the runtime controller alone
with it, and SHALL state that the controller is throughput- and latency-neutral
against the static default it replaced.

#### Scenario: The improvement is attributed correctly

- **WHEN** the docs state the out-of-the-box throughput or latency improvement
- **THEN** they name the pre-change baseline it is measured against and do not present the controller as its sole cause

#### Scenario: The controller's neutrality is stated

- **WHEN** the docs describe what the controller contributes
- **THEN** they state that it adapts the allowance within its cap and refuses to grow under CPU saturation, and that its own effect on throughput and latency is parity

#### Scenario: The guardrails are shown

- **WHEN** the docs present the adaptive-capacity figures
- **THEN** they state that replies are byte-identical under the interleaved A/B harness, that the Ruby client suite passes against the same server, and that `EMB.READY` answers while the controller is active
