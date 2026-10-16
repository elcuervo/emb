# Spec Delta

## MODIFIED Requirements

### Requirement: CI executes the CGo-dependent suites

Continuous integration SHALL execute the CGo-dependent package tests on pull requests, with the ONNX runtime and the model fixture available, so measured coverage is enforced rather than local-only. The same packages SHALL additionally be executed under Go's race detector in the same job and on the same pull request, so a data race in the served path fails the build. A test whose fixture work is bounded by a wall-clock deadline SHALL take the server's test-only deadline seam rather than relying on the machine being fast enough for an instrumented build.

#### Scenario: CGo suites run on a pull request

- **WHEN** a pull request changes Go code in the CGo-dependent packages
- **THEN** CI SHALL run the tests for the server, registry, pipeline, ONNX, script, image-processing, and emb-top packages
- **AND** a failing test in any of them SHALL fail the job

#### Scenario: The race detector covers the served path

- **WHEN** the CGo test job runs
- **THEN** the server, registry, pipeline, ONNX, script and admission packages SHALL also run with the race detector enabled
- **AND** a reported data race SHALL fail the job

#### Scenario: Fixture is available to CI

- **WHEN** the CGo test job runs
- **THEN** the ONNX runtime and the model fixture the tests require SHALL be present, so model-dependent tests execute rather than skip
