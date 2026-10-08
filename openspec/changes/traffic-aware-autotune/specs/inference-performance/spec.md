# Spec Delta

## MODIFIED Requirements

### Requirement: Concurrent multi-model session tuning

The server SHALL allow tuning ONNX session execution (execution mode and intra-op thread count) per model so two models (e.g. `siglip2` and `e5`) serving concurrently do not unnecessarily contend for CPU, without changing the returned embeddings. When `workers` or `intra_op_threads` are unset, the server SHALL derive them from the process's effective CPU and memory budget (the container/cgroup limit when present, not the host's), keeping the total intra-op thread budget at or below the effective core count.

#### Scenario: Two models served concurrently

- **WHEN** an `EMB.MULTI` embeds the same text through two models
- **THEN** both models SHALL return correct, length-`dim*4` embeddings
- **AND** session options SHALL be configurable per model (execution mode and `intra_op_threads`)

#### Scenario: Derivation uses the container budget

- **WHEN** `workers` and `intra_op_threads` are unset inside a container whose memory limit is smaller than the host's
- **THEN** the derived worker and thread counts SHALL be based on the container's CPU and memory limits

## ADDED Requirements

### Requirement: Adaptive concurrency for unbatched pools

An unbatched embedding pool's effective in-flight concurrency SHALL adapt to observed traffic within its configured cap, and adapting SHALL NOT change the returned embeddings. Batcher (single-session) pools SHALL be excluded.

#### Scenario: Concurrency grows under burst

- **WHEN** an unbatched pool's in-flight reaches its current allowance while CPU headroom exists
- **THEN** its allowance SHALL increase, up to the configured cap

#### Scenario: Embeddings are unchanged by adaptation

- **WHEN** the same texts are embedded before and after the allowance changes
- **THEN** the returned embeddings SHALL be byte-identical
