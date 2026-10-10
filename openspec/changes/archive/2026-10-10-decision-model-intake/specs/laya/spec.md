# Spec Delta

## ADDED Requirements

### Requirement: Mounted decision graph declares only the outputs it is asked for

A mounted decision graph SHALL be usable when it declares the five decision inputs and the
outputs the preset requests, even if it omits other outputs of the reference export.
`scripts/laya.lua` requests `logits` and `act_logits`; `last_hidden_state` is NOT required,
and its absence SHALL NOT fail a request or change an answer.

#### Scenario: A reduced export answers

- **WHEN** a model entry mounts an export that declares `input_ids`, `attention_mask`, `marker_pos`, `marker_mask`, `qtype`, `logits`, and `act_logits`, and does not declare `last_hidden_state`
- **THEN** the model SHALL load, `EMB.EVSHA` SHALL answer every question, and the reply SHALL carry the same payload shapes as the reference export

#### Scenario: A missing requested output fails loudly

- **WHEN** a mounted graph does not declare `logits` or `act_logits`
- **THEN** the reply SHALL be a run error naming the missing output, and no answer payload SHALL be returned

#### Scenario: An unpublished checkpoint ships its envelope in config

- **WHEN** a third-party export ships no `rl_agent_config.json` / `onnx_config.json`
- **THEN** the checkpoint envelope SHALL be supplied from the model entry's `scripts` config, and boot SHALL NOT read the checkpoint's own files
