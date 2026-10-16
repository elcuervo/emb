# Spec Delta

## MODIFIED Requirements

### Requirement: No leaks under sustained load or failure

Scripted evaluation SHALL NOT leak goroutines, sessions, tensors or heap across sustained traffic, repeated model lifecycles, or failed evaluations:

- Running a large number of scripted evaluations SHALL leave the goroutine count at its baseline and heap growth flat after garbage collection.
- Repeatedly opening and closing a scripted model SHALL NOT grow the resource count.
- An evaluation that fails for any reason — deadline exceeded, tensor budget exceeded, unknown output name, graph error, or a script-level error raised after `emb.run` — SHALL release everything it allocated.
- An evaluation whose input assembly fails SHALL release the values it had already created for that run: a duplicated or missing input name, and a tensor whose spec fails validation, are failures of the same obligation as a graph error, whether or not the ORT run is ever reached. An input name the graph does not declare is ignored rather than refused, so it surfaces as the missing input it displaced.
- A partially-constructed resource (for example a session pool that failed partway through) SHALL NOT leave already-created sessions open.

#### Scenario: Sustained scripted traffic does not leak

- **WHEN** a few thousand scripted evaluations run across persistent connections
- **THEN** the goroutine count returns to the baseline (within a small tolerance) and heap growth is flat after garbage collection

#### Scenario: Failed evaluation releases its resources

- **WHEN** an evaluation raises after a successful `emb.run` (for example a script error during decoding)
- **THEN** the output tensors allocated for that run are released and a subsequent evaluation is unaffected

#### Scenario: A failed input assembly releases what it built

- **WHEN** a scripted run supplies inputs for a graph with more than one input and one of them is missing, duplicated or invalid, after an earlier one was already built
- **THEN** the values created for the earlier inputs SHALL be destroyed before the error is returned
- **AND** repeating that failing run at scale SHALL NOT grow the process's resident or ORT-allocated memory beyond the documented tolerance

#### Scenario: Partial construction failure closes what it opened

- **WHEN** creating a scripted session pool fails after some sessions were opened
- **THEN** the sessions already opened are closed before the error is returned

#### Scenario: Repeated lifecycle does not accumulate

- **WHEN** a scripted model is created, exercised and closed repeatedly
- **THEN** the resources retained after each cycle return to the pre-cycle baseline

## ADDED Requirements

### Requirement: A reused output tensor is never handed out after destruction

A session that reuses one output tensor across runs SHALL NOT return a slice belonging to a tensor it has destroyed. When replacing the reused tensor fails, the session SHALL either re-create it on the next run or report the failure, and SHALL NOT treat the destroyed tensor as still valid.

#### Scenario: A failed re-allocation is not mistaken for a reused buffer

- **WHEN** a session destroys its reused output tensor to re-create it for a new shape and the creation fails
- **THEN** the next run with that shape SHALL create the tensor again or fail with an error
- **AND** it SHALL NOT return data from the destroyed tensor, and SHALL NOT leave a tensor value behind for the caller to read
