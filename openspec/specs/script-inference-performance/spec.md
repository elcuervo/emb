# script-inference-performance

## Purpose

Makes scripted inference parallel and batched: per-model session pools for concurrent requests, one padded run per multi-text request via `emb.run_batch`, and compiled-script reuse for repeat evaluations.

## Requirements

### Requirement: Parallel scripted execution via session pool

The server SHALL support multiple named-tensor sessions per model (`script_workers`) so concurrent scripted evaluations distribute across sessions instead of serializing on one. Unset or 0 SHALL auto-tune by available RAM and model weight size, mirroring the embedding worker pool. Evaluations SHALL remain pure compute and deterministic; reply shape and caching are unchanged.

#### Scenario: Concurrent distinct requests run in parallel

- **WHEN** a model has `script_workers: 4` and four connections send distinct-text `EMB.EVSHA` requests concurrently
- **THEN** the four evaluations complete concurrently (bounded by the session count and machine cores), not queueing on a single session

#### Scenario: Auto-tuned pool size

- **WHEN** `script_workers` is unset (0)
- **THEN** the pool size is computed from available RAM and model size, with a minimum of 1

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