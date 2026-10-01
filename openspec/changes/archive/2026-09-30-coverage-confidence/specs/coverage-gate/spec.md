# Spec Delta

## Purpose

Defines the coverage bar for emb's production packages and requires it to be measured honestly, enforced in CI, and validated by the project's performance tests and system tests.

## ADDED Requirements

### Requirement: Cross-package coverage measurement

The project SHALL measure test coverage with cross-package instrumentation so a package exercised only through another package's tests is credited, and SHALL report the total together with the documented exclusions.

#### Scenario: Coverage target measures cross-package

- **WHEN** the coverage target is run
- **THEN** it SHALL use cross-package instrumentation (`-coverpkg=./...`) and report the merged total
- **AND** a package exercised through a sibling package's tests SHALL be credited in that total

#### Scenario: Exclusions are documented

- **WHEN** the coverage total is reported
- **THEN** the command-line entrypoints and the benchmark harness SHALL be named as exclusions and excluded from the production total
- **AND** the production total SHALL be reported separately from the whole-repository total

### Requirement: Coverage floor for production packages

The project SHALL enforce a recorded coverage floor per production package, failing the gate when a package falls below its floor.

#### Scenario: Package below floor fails the gate

- **WHEN** a production package's cross-package coverage is below the floor recorded for it
- **THEN** the coverage gate SHALL exit non-zero and name the package, its coverage, and its floor

#### Scenario: Package at or above floor passes

- **WHEN** every production package is at or above its recorded floor
- **THEN** the coverage gate SHALL exit zero

### Requirement: CI executes the CGo-dependent suites

Continuous integration SHALL execute the CGo-dependent package tests on pull requests, with the ONNX runtime and the model fixture available, so measured coverage is enforced rather than local-only.

#### Scenario: CGo suites run on a pull request

- **WHEN** a pull request changes Go code in the CGo-dependent packages
- **THEN** CI SHALL run the tests for the server, registry, pipeline, ONNX, script, image-processing, and emb-top packages
- **AND** a failing test in any of them SHALL fail the job

#### Scenario: Fixture is available to CI

- **WHEN** the CGo test job runs
- **THEN** the ONNX runtime and the model fixture the tests require SHALL be present, so model-dependent tests execute rather than skip

### Requirement: Skipped tests are accounted for

The test run SHALL report the number of skipped or fixture-gated tests, so a passing run is distinguishable from a run in which the model-dependent tests did not execute.

#### Scenario: Absent fixture is reported

- **WHEN** a model- or fixture-dependent test is skipped because its fixture is absent
- **THEN** the run SHALL report the skip count (or a summary naming the skipped tests)
- **AND** the job SHALL NOT present the run as fully executed

### Requirement: Coverage work is validated by performance tests

The coverage additions SHALL be validated by the project's performance harnesses: the benchmark baseline SHALL remain within the documented noise gate, and the test suite's wall-clock SHALL remain within its recorded budget.

#### Scenario: Benchmark baseline unchanged

- **WHEN** the performance harness runs against the build carrying the new tests and compares to the recorded baseline
- **THEN** every cell SHALL remain within the documented noise tolerance (req/s within the configured bound, p50 within the configured bound)

#### Scenario: Suite wall-clock bounded

- **WHEN** the full test suite runs
- **THEN** its wall-clock SHALL remain within the recorded budget for the suite, and the new tests SHALL NOT require a network download or a live model beyond the existing fixture

### Requirement: Coverage work is validated by system tests

The coverage additions SHALL be accepted only when the project's system tests pass against a running server.

#### Scenario: System test suite passes

- **WHEN** the coverage change is validated
- **THEN** the end-to-end suite (Go server tests plus the Ruby client suite via the unified runner) SHALL pass
- **AND** the embedding reference checks and the multi-model byte-equality check SHALL pass
- **AND** both gems SHALL build and validate

#### Scenario: No production behavior change

- **WHEN** the coverage change is validated
- **THEN** the existing system tests SHALL pass unmodified, and no wire-format or configuration behavior SHALL change
