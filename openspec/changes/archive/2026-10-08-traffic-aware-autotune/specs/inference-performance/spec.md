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
