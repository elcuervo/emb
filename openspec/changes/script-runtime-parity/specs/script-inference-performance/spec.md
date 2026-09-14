# script-inference-performance delta

## MODIFIED Requirements

### Requirement: Parallel scripted execution via session pool

The server SHALL support multiple named-tensor sessions per model (`script_workers`) so concurrent scripted evaluations distribute across sessions instead of serializing on one. Scripted sessions are a **separate** pool from the embedding path's, so their default size SHALL be bounded by the number of inference sessions the model's embedding pool actually holds — one for a batcher pool (which is the default), one per worker for a worker pool — so that scripting never adds more model instances than the embedding path already committed to. Unset or 0 SHALL auto-tune by available RAM and model weight size, then apply that bound, with a minimum of 1. An explicitly configured `script_workers` SHALL be honoured verbatim as an operator override and SHALL NOT be silently clamped; raising it trades memory for scripted parallelism and SHALL be documented as such.

Named-tensor sessions and the script tokenizer SHALL be created lazily on the first evaluation that actually invokes `emb.run`/`emb.run_batch`; evaluations that only use `emb.embed` SHALL NOT open them, and SHALL reuse the embedding pool's tokenizer. Evaluations SHALL remain pure compute and deterministic; reply shape and caching are unchanged.

#### Scenario: Auto-tuned pool size is bounded by the batcher pool

- **WHEN** `script_workers` is unset (0), batching is enabled (the default, so the embedding pool holds one session), and a script that calls `emb.run` is evaluated
- **THEN** exactly one named-tensor session is opened for that model

#### Scenario: Auto-tuned pool size is bounded by the embedding pool

- **WHEN** `script_workers` is unset (0) and the model's embedding pool holds two worker sessions
- **THEN** the script session count SHALL be at most 2, regardless of available RAM

#### Scenario: Auto-tuned pool size

- **WHEN** `script_workers` is unset (0)
- **THEN** the pool size is computed from available RAM and model size, with a minimum of 1, and clamped to the number of sessions the model's embedding pool actually holds

#### Scenario: Explicit override is honoured

- **WHEN** a model sets `script_workers: 4` while its embedding pool holds one session
- **THEN** exactly four scripted named-tensor sessions are opened, and the count is observable in the model's reported script footprint

#### Scenario: Concurrent distinct requests run in parallel

- **WHEN** a model has `script_workers: 4` and four connections send distinct-text `EMB.EVSHA` requests concurrently
- **THEN** the four evaluations complete concurrently (bounded by the session count and machine cores), not queueing on a single session

#### Scenario: Embed-only scripts open no named-tensor sessions

- **WHEN** a script that calls only `emb.embed` is evaluated on a model that has never run `emb.run`
- **THEN** the model's named-tensor script sessions remain unopened and no second tokenizer is loaded

#### Scenario: Lazy creation is observable

- **WHEN** a script that returns a constant is evaluated
- **THEN** no named-tensor session is created for that model

### Requirement: Scripted inference parity budgets

Scripted evaluation SHALL stay within a bounded factor of the equivalent native embedding work, so scripts are a production-viable path rather than a fallback. The budgets below SHALL be enforced by committed benchmarks run as part of the standard benchmark suite, with a captured baseline; a regression beyond 10% of the recorded value SHALL fail the benchmark gate.

**Latency parity (embedding class).** A script that embeds texts with `emb.embed` and scores them with `emb.similarity` SHALL complete within **1.30×** the wall-clock time of an `EMB` command embedding the same texts, measured at 8, 32, and 128 tokens with a cold cache and a single connection.

**Metal parity (overhead share).** The non-inference overhead of a scripted evaluation — interpreter startup, host-call dispatch, tensor marshalling, and reply conversion — SHALL be a small bounded fraction of the inference it wraps, measured against the same graph run performed with no host-side reduction:

- Embedding class: scripted total SHALL be at most **1.30×** the native command at 8 tokens, and at most **1.15×** at 32 and 128 tokens.
- Raw-tensor class: a script that runs the graph and performs no reduction SHALL be at most **1.20×** a bare graph run; adding a packed read plus a host reduction SHALL add at most **0.35×** over that.

**Materialization tax (raw-tensor class).** For a graph whose output is `[1, 128, 384]`, a script that requests packed output, reads it, and reduces it with host operations SHALL take no more than **1.35×** the time of the same graph run that requests no outputs. The non-inference overhead attributable to output materialization SHALL NOT exceed **0.02 µs per output element**.

**Memory.** A scripted evaluation SHALL NOT add more than one named-tensor model session beyond what the embedding path already holds, and SHALL NOT grow RSS by more than a model footprint per session it does open. In particular:

- Embedding-class scripts (`emb.embed`) and scripts that open no model session SHALL stay within **15%** of the model's loaded footprint (the embedding pool is shared, not duplicated).
- A raw-tensor script (`emb.run`) on a batcher-pool model SHALL open at most **one** named-tensor session where it previously opened one per auto-tuned worker (~10), i.e. RSS grows by at most roughly one model footprint instead of multiplying it.

**Throughput scaling.** Scripted evaluations SHALL use their session pool: with N script sessions available, sustained scripted throughput SHALL reach at least **0.85 × N** times single-session throughput, up to the machine's core count. The recorded baseline ratio is the regression gate; the absolute 0.85 target is asserted on a quiet reference host.

**Scaling.** The non-inference portion of scripted evaluation time SHALL grow no faster than linearly with output element count, and the per-element coefficient SHALL meet the materialization bound above at both 32 and 128 tokens.

#### Scenario: Embedding parity at three sequence lengths

- **WHEN** the parity benchmark runs an `emb.embed` + `emb.similarity` script and the equivalent `EMB` command at 8, 32, and 128 tokens
- **THEN** the 8-token measurement is at most 1.30× native and the 32- and 128-token measurements are at most 1.15× native

#### Scenario: Bare scripted graph run stays close to the graph

- **WHEN** a script runs a graph and returns without reducing its output, and the same graph is run directly
- **THEN** the scripted run is at most 1.20× the direct run

#### Scenario: Reading a large output is no longer the dominant cost

- **WHEN** the materialization benchmark runs the graph with packed output plus a host reduction, and the same graph with no output requested
- **THEN** the ratio is at most 1.35×, and the derived per-element overhead is at most 0.02 µs

#### Scenario: Every default configuration stays within the memory budget

- **WHEN** a model with batching enabled serves `EMB` and then a single `emb.run` script, and RSS is sampled after the first scripted evaluation
- **THEN** the increase over the post-`EMB` RSS is at most one model footprint (the single additional named-tensor session), and the reported script session count does not exceed the embedding pool's session count

#### Scenario: Scripted throughput scales with the pool

- **WHEN** a model is configured with four script sessions and scripted evaluations are driven concurrently
- **THEN** sustained throughput is at least 0.85 × 4 times the single-session throughput, up to the core count (asserted absolutely on a reference host; the recorded baseline ratio is the regression gate elsewhere)

#### Scenario: Embed-only script does not duplicate model memory

- **WHEN** a server that has served only scripted `emb.embed` requests is sampled for RSS after the first evaluation
- **THEN** the increase over the pre-evaluation RSS is at most 15% of the model's loaded footprint

#### Scenario: Session count respects the embedding pool

- **WHEN** `script_workers` is unset and the model's embedding pool holds one session (the batching default)
- **THEN** at most one named-tensor session is opened, observable through the script resource footprint reported in stats

#### Scenario: Budgets are regression-gated

- **WHEN** the standard benchmark suite runs in CI
- **THEN** the parity, metal-parity, materialization, memory and throughput benchmarks execute and compare against the captured baseline, failing on a regression greater than 10%
