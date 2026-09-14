# script-inference-performance

## Purpose

Makes scripted inference parallel and batched: per-model session pools for concurrent requests, one padded run per multi-text request via `emb.run_batch`, and compiled-script reuse for repeat evaluations.

## Requirements

### Requirement: Parallel scripted execution via session pool

The server SHALL support multiple named-tensor sessions per model (`script_workers`) so concurrent scripted evaluations distribute across sessions instead of serializing on one. Unset or 0 SHALL auto-tune by available RAM and model weight size, bounded by the embedding pool's worker count for the same model, so a scripted model never opens more sessions than the embedding path. Named-tensor sessions and the script tokenizer SHALL be created lazily on the first evaluation that actually invokes `emb.run`/`emb.run_batch`; evaluations that only use `emb.embed` SHALL NOT open them, and SHALL reuse the embedding pool's tokenizer. Evaluations SHALL remain pure compute and deterministic; reply shape and caching are unchanged.

#### Scenario: Concurrent distinct requests run in parallel

- **WHEN** a model has `script_workers: 4` and four connections send distinct-text `EMB.EVSHA` requests concurrently
- **THEN** the four evaluations complete concurrently (bounded by the session count and machine cores), not queueing on a single session

#### Scenario: Auto-tuned pool size is bounded by the embedding pool

- **WHEN** `script_workers` is unset (0) and the model's embedding pool has 2 workers
- **THEN** the script session count SHALL be at most 2, regardless of available RAM

#### Scenario: Auto-tuned pool size

- **WHEN** `script_workers` is unset (0)
- **THEN** the pool size is computed from available RAM and model size, with a minimum of 1, and clamped to the model's embedding pool worker count

#### Scenario: Embed-only scripts open no named-tensor sessions

- **WHEN** a script that calls only `emb.embed` is evaluated on a model that has never run `emb.run`
- **THEN** the model's named-tensor script sessions remain unopened and no second tokenizer is loaded

#### Scenario: Lazy creation is observable

- **WHEN** a script that returns a constant is evaluated
- **THEN** no named-tensor session is created for that model

### Requirement: Batched scripted inference (`emb.run_batch`)

The server SHALL provide an `emb.run_batch(inputsArray)` host block: an array of input-spec tables (each `{name = {shape, data}}`) SHALL be merged into one padded inference run and returned as an array of per-item named-output maps, one per input. The host SHALL validate that all items declare the same tensor names, ranks, and dtypes; padding SHALL extend each named tensor to the maximum dimensions across the batch (zero-filled), and the script remains responsible for any masking semantics.

When a scripted evaluation is requested with multiple texts, the server SHALL evaluate the script ONCE with all texts in KEYS (Redis semantics), and the script SHALL return one value per text: the value itself for a single text, or an array whose elements correspond 1:1 to the texts in order. A multi-text result that is not such an array SHALL be an error reply.

#### Scenario: Multi-text request runs one inference call

- **WHEN** a script passes two input specs to `emb.run_batch`
- **THEN** the underlying session is invoked once for both items, and the result contains one output map per input

#### Scenario: Multi-text evaluation passes all texts as KEYS

- **WHEN** `EMB.EVSHA` is sent with two texts and a script that returns an array of per-text hash values
- **THEN** the script executes once with `KEYS = {text1, text2}` and the reply is an array of the two hashes

#### Scenario: Deterministic batch results

- **WHEN** any single item from a batch is rerun alone via `emb.run`
- **THEN** its outputs match the corresponding batch item's outputs (padding does not change per-item semantics)

### Requirement: Compiled script reuse

The server SHALL compile a script's source once per (model, SHA) and reuse the compiled bytecode for subsequent evaluations of the same SHA; each evaluation SHALL still run in a fresh Lua state with the same sandbox and budgets. `EMB.SCRIPT FLUSH` SHALL invalidate compiled bytecode caches alongside source caches.

#### Scenario: Repeat EVSHA does not recompile

- **WHEN** the same script is executed twice from the script cache
- **THEN** the second execution reuses the cached bytecode (observable via the engine's compile counter) and produces identical replies

#### Scenario: Flush clears compiled bytecode

- **WHEN** `EMB.SCRIPT FLUSH` is issued
- **THEN** subsequent evaluations compile anew and still produce identical replies

### Requirement: Scripted inference parity budgets

Scripted evaluation SHALL stay within a bounded factor of the equivalent native embedding work, so scripts are a production-viable path rather than a fallback. The budgets below SHALL be enforced by committed benchmarks run as part of the standard benchmark suite, with a captured baseline; a regression beyond 10% of the recorded value SHALL fail the benchmark gate.

**Latency parity (embedding class).** A script that embeds texts with `emb.embed` and scores them with `emb.similarity` SHALL complete within **1.30×** the wall-clock time of an `EMB` command embedding the same texts, measured at 8, 32, and 128 tokens with a cold cache and a single connection.

**Materialization tax (raw-tensor class).** For a graph whose output is `[1, 128, 384]`, a script that requests packed output, reads it, and reduces it with host operations SHALL take no more than **1.35×** the time of the same graph run that requests no outputs. The non-inference overhead attributable to output materialization SHALL NOT exceed **0.02 µs per output element**.

**Memory.** The first scripted evaluation of an embed-only workload SHALL increase process RSS by no more than **15%** of the model's loaded footprint. A model's scripted session count SHALL NOT exceed its embedding pool worker count.

**Scaling.** The non-inference portion of scripted evaluation time SHALL grow no faster than linearly with output element count, and the per-element coefficient SHALL meet the materialization bound above at both 32 and 128 tokens.

#### Scenario: Embedding parity at three sequence lengths

- **WHEN** the parity benchmark runs an `emb.embed` + `emb.similarity` script and the equivalent `EMB` command at 8, 32, and 128 tokens
- **THEN** each scripted measurement is at most 1.30× the corresponding native measurement

#### Scenario: Reading a large output is no longer the dominant cost

- **WHEN** the materialization benchmark runs the graph with packed output plus a host reduction, and the same graph with no output requested
- **THEN** the ratio is at most 1.35×, and the derived per-element overhead is at most 0.02 µs

#### Scenario: Embed-only script does not duplicate model memory

- **WHEN** a server that has served only scripted `emb.embed` requests is sampled for RSS after the first evaluation
- **THEN** the increase over the pre-evaluation RSS is at most 15% of the model's loaded footprint

#### Scenario: Session count respects the embedding pool

- **WHEN** `script_workers` is unset and the model's embedding pool has 1 worker
- **THEN** at most one named-tensor session is opened, observable through the script resource footprint reported in stats

#### Scenario: Budgets are regression-gated

- **WHEN** the standard benchmark suite runs in CI
- **THEN** the parity, materialization, and memory benchmarks execute and compare against the captured baseline, failing on a regression greater than 10%
