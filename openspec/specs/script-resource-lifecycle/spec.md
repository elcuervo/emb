## Purpose

Owns the lifetime of everything a scripted evaluation allocates — named-tensor sessions, the tokenizer shared with the embedding pool, cached output tensors, and the script source/bytecode caches — so scripting is memory-bounded and provably leak-free under sustained load and repeated model lifecycles.

## ADDED Requirements

### Requirement: Scripted resources are acquired lazily

Every resource the scripted path needs SHALL be created only when a host function that actually requires it first runs, never when a scripted evaluation begins:

- Named-tensor sessions and the script tokenizer SHALL be created on the first `emb.run`/`emb.run_batch` (or `emb.tokenize.*`) call within an evaluation.
- Image resources SHALL be created on the first `emb.image.preprocess` / `emb.image.info` / `emb.image.embed` call.
- A script that uses none of these SHALL allocate none of them.
- The reported scripted footprint for a model SHALL be zero for each surface the model has never exercised.
- The model's reported footprint SHALL expose the named-tensor session count (`script_sessions`), whether the shared script tokenizer is loaded (`script_tokenizer`), and the image session count (`image_sessions`), each without creating the resource it reports.

#### Scenario: Constant script allocates nothing

- **WHEN** a script that returns a constant is evaluated on a model that has served only such scripts
- **THEN** no named-tensor session, no script tokenizer and no image session exists for that model

#### Scenario: Embed-only script opens no raw-tensor resources

- **WHEN** a script that calls only `emb.embed` is evaluated
- **THEN** the model's named-tensor script sessions remain unopened

#### Scenario: Image resources are not opened for non-image scripts

- **WHEN** a script that never calls `emb.image.*` is evaluated on a model that has an image configuration
- **THEN** the model's image sessions remain unopened

### Requirement: Every scripted resource has a single owner and is released

Each resource allocated by the scripted path SHALL have exactly one owner and SHALL be released by that owner on eviction or on server shutdown (which closes the model registry):

- Named-tensor sessions SHALL be closed when their model is closed.
- The tokenizer shared between the embedding pool and the scripted path SHALL be closed exactly once, by the model that owns it, and SHALL NOT be closed by a component that merely borrowed it.
- Cached output tensors SHALL be destroyed when evicted from the cache and when their session is closed.
- Closing a model twice, or closing a model whose scripted resources were never created, SHALL be safe and SHALL NOT close anything twice.

#### Scenario: Closing a model releases scripted sessions

- **WHEN** a model has served `emb.run` evaluations and the registry is closed
- **THEN** every named-tensor session created for scripting is closed, and a subsequent close of the same model is a no-op

#### Scenario: The shared tokenizer is closed once

- **WHEN** a model has served both `EMB` requests and scripted evaluations, and the registry is closed
- **THEN** the shared tokenizer is released exactly once despite being referenced by both paths

#### Scenario: Lazily-created resources close safely

- **WHEN** a model whose scripted resources were never created is closed
- **THEN** the close succeeds without error and without creating anything

### Requirement: Scripted caches are bounded

Every cache the scripted path maintains SHALL be bounded by a documented constant, and eviction SHALL release the evicted resource rather than merely unlinking it:

- The per-session cached output tensors SHALL be bounded by a fixed number of distinct input-shape signatures, and an evicted entry SHALL have its tensors destroyed.
- The script source cache and the compiled-bytecode cache SHALL remain bounded per model, as they are today.
- Adversarial input SHALL NOT grow a cache without bound: many distinct scripts, many distinct tensor shapes, and many distinct images SHALL each be bounded by their cache's cap.

#### Scenario: Output-tensor cache is bounded

- **WHEN** a scripted evaluation runs repeatedly with many distinct input shapes on one session
- **THEN** the number of retained output sets never exceeds the documented cap

#### Scenario: Eviction destroys tensors

- **WHEN** an output-tensor cache entry is evicted
- **THEN** the tensors it held are destroyed rather than left for the garbage collector

#### Scenario: Distinct scripts stay bounded

- **WHEN** a client loads far more distinct scripts than the cache cap for one model
- **THEN** the cache retains at most its cap, and the evicted source and bytecode are dropped

### Requirement: No leaks under sustained load or failure

Scripted evaluation SHALL NOT leak goroutines, sessions, tensors or heap across sustained traffic, repeated model lifecycles, or failed evaluations:

- Running a large number of scripted evaluations SHALL leave the goroutine count at its baseline and heap growth flat after garbage collection.
- Repeatedly opening and closing a scripted model SHALL NOT grow the resource count.
- An evaluation that fails for any reason — deadline exceeded, tensor budget exceeded, unknown output name, graph error, or a script-level error raised after `emb.run` — SHALL release everything it allocated.
- A partially-constructed resource (for example a session pool that failed partway through) SHALL NOT leave already-created sessions open.

#### Scenario: Sustained scripted traffic does not leak

- **WHEN** a few thousand scripted evaluations run across persistent connections
- **THEN** the goroutine count returns to the baseline (within a small tolerance) and heap growth is flat after garbage collection

#### Scenario: Failed evaluation releases its resources

- **WHEN** an evaluation raises after a successful `emb.run` (for example a script error during decoding)
- **THEN** the output tensors allocated for that run are released and a subsequent evaluation is unaffected

#### Scenario: Partial construction failure closes what it opened

- **WHEN** creating a scripted session pool fails after some sessions were opened
- **THEN** the sessions already opened are closed before the error is returned

#### Scenario: Repeated lifecycle does not accumulate

- **WHEN** a scripted model is created, exercised and closed repeatedly
- **THEN** the resources retained after each cycle return to the pre-cycle baseline

### Requirement: Leak guarantees are enforced, not asserted

The guarantees above SHALL be enforced by automated tests using the project's existing heap and goroutine probes, so a regression fails the test suite rather than needing manual RSS watching. Timing-sensitive variants SHALL NOT introduce `time.Sleep`-based assertions.

#### Scenario: Automated leak gate

- **WHEN** the test suite runs the scripted-runtime leak gate
- **THEN** it measures goroutine count and in-use heap before and after sustained scripted traffic, and across open/close cycles, and fails on growth beyond the documented tolerance
