# Spec Delta

## ADDED Requirements

### Requirement: The Lua section points at the complete reference

The docs surface's Lua scripting section SHALL remain the narrative introduction
— what a script is, one worked example, and the loading-and-calling shape — and
SHALL link to the scripting reference surface for the complete host API, tensor
spec, reply grammar, budgets, and cache identity. It SHALL NOT present a second,
partial copy of the complete form, so a scripting fact has one complete home.

#### Scenario: The docs section links to the reference

- **WHEN** a reader opens the docs surface's Lua scripting section
- **THEN** it links to the scripting reference surface and states that the
  reference carries the complete host surface

#### Scenario: The docs do not fork the reference

- **WHEN** a host function or budget is added to the reference
- **THEN** the docs section is not required to reproduce it, and the two cannot
  contradict each other because the docs defer to the reference
