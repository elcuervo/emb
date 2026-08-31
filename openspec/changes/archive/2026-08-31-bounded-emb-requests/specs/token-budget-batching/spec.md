# token-budget-batching Specification

## REMOVED Requirements

### Requirement: Single-request batches are never split

A single `EMB` command's texts SHALL be processed in one run, even when their tokens exceed the budget.

#### Scenario: Oversized single command

- **WHEN** a single `EMB` command carries more real tokens than the budget
- **THEN** the server SHALL run it as a single inference and return all its embeddings

## ADDED Requirements

### Requirement: Oversized commands are truncated at admission

A single `EMB` command exceeding the configured `max_texts` cap SHALL be truncated to its first `max_texts` texts before any cache lookup or inference, with overflow reply slots as `null`; commands within the cap SHALL be processed as before.

#### Scenario: Within-cap oversized-token command runs as one inference

- **WHEN** a single `EMB` command carries more real tokens than the window budget but has fewer texts than `max_texts`
- **THEN** the server SHALL run it as a single inference and return all its embeddings (behavior unchanged from before this change)

#### Scenario: Over-cap command is truncated

- **WHEN** a single `EMB` command carries more texts than `max_texts`
- **THEN** the server SHALL infer only the first `max_texts` texts
- **AND** SHALL reply with one slot per requested text, overflow slots `null`
- **AND** SHALL NOT tokenize, cache-lookup, or infer the overflow texts