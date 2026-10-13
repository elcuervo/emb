# Spec Delta

## ADDED Requirements

### Requirement: Fractional bandwidth saturation accounting

CPU saturation SHALL be normalized using fractional effective CPU bandwidth from the common resource-resolution contract and actual monotonic elapsed sample time, not a rounded admission-token count or assumed ticker interval. Unavailable accounting SHALL NOT authorize concurrency growth. Existing explicit caps, damping, and runtime no-reshape guarantees SHALL remain in effect.

#### Scenario: Sub-one-CPU saturation
- **WHEN** a busy model consumes 0.5 CPU-seconds over one elapsed second under a 0.5-CPU bandwidth limit
- **THEN** its process CPU utilization SHALL be treated as fully utilized rather than 50 percent idle
- **AND** concurrency SHALL NOT grow on a claim of CPU headroom

#### Scenario: Delayed sampling
- **WHEN** scheduler delay causes two elapsed seconds between observations
- **THEN** CPU normalization SHALL use those two seconds rather than a nominal one-second ticker interval

#### Scenario: Accounting unavailable
- **WHEN** usable CPU accounting is unavailable for a sampling window
- **THEN** the controller SHALL retain its allowance without claiming measured headroom

#### Scenario: Layout remains fixed
- **WHEN** fractional CPU accounting changes a traffic classification
- **THEN** the controller SHALL NOT rebuild native sessions or change their thread width
