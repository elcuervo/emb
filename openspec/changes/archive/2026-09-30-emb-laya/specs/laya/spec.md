# laya Specification

## Purpose
Serves Laya decision checkpoints — the same ONNX exports the ruby-laya gem consumes (`codenamev/laya-onnx`) — over the scripted-model path, so clients get typed, calibrated answers over the existing `EMB.EVAL`/`EMB.EVSHA` surface.

## ADDED Requirements

### Requirement: Laya checkpoint mounting

A model config entry SHALL be able to mount a Laya export directory with `onnx:` + `tokenizer:` (as any scripted model) plus `script_preload`, and the preset registered via the `scripts:` field. The graph SHALL be addressable by `EMB.EVAL`/`EMB.EVSHA` exactly like other scripted models; its five inputs (`input_ids`, `attention_mask`, `marker_pos`, `marker_mask`, `qtype`) are the contract the preset feeds, and a graph that does not declare them SHALL fail the request with a run error naming the missing input. The checkpoint's budgets and temperatures (from `rl_agent_config.json` / `onnx_config.json`) are supplied as the preset's config: normally on the model entry via the `scripts` entry's `config` (exposed to the script as `emb.script.config`, see the `script-config` capability), and optionally per request as an override; boot does not read the checkpoint's files.

#### Scenario: Mounted checkpoint answers an EVAL

- **WHEN** a config entry mounts a Laya export with `script_preload: true` and the server reports ready
- **THEN** `EMB.SCRIPT EXISTS <model> <laya-sha>` replies `[1]` and `EMB.EVSHA <model> <laya-sha>` executes the preset without a client-side `EMB.SCRIPT LOAD`

#### Scenario: Non-Laya graph fails at request time

- **WHEN** a request targets a graph that does not declare all five Laya inputs
- **THEN** the reply is a run error naming the missing input, and no answer payload is returned

### Requirement: Typed questions over EVAL

`EMB.EVAL <model> <laya-script> 1 <state> <questions-json> [<config-json>]` SHALL answer every question in one forward pass. `state` is a single text (serialized state; a JSON object or array must be sent in Python-style JSON with spaces after every comma and colon, because the checkpoints were trained on exactly those strings). `questions-json` SHALL be a JSON object of question id → definition; each definition has a `type` of `choice` | `score` | `noul`, an `instructions` string, and criteria following the Ruby gem's schema (`choice` labels→descriptions Hash or Array of labels; `score` Array of level descriptions in order; `noul` optional `true`/`false` descriptions). Criteria label ORDER SHALL be significant — marker order maps to label order — so criteria objects are read in document order. `config-json` is optional and SHALL accept the checkpoint's budgets and temperatures: `max_len`, `head_max_len`, `min_seq`, `min_markers`, `temperature` (choice/score/noul), `temperature_by_options` (bucket → value). When the model entry declares those values (see the `script-config` capability), the call MAY omit `config-json`, and any value sent there overrides the model entry's. An empty questions object SHALL reply with an empty answers object. A malformed definition (unknown type, missing instructions, empty criteria) SHALL reply with an error and SHALL NOT run inference.

#### Scenario: Three question types in one pass

- **WHEN** a request carries one `choice`, one `score`, and one `noul` question and returns a single model batch
- **THEN** the reply carries one answer per question id, each with the payload shape of its type (see answer contract), and `input_tokens` counts the tokenizer output

#### Scenario: Invalid question is rejected before inference

- **WHEN** a question definition has an unknown `type` or a `choice` with empty criteria
- **THEN** the reply is an error and no model run happens for that request

### Requirement: Answer contract

Each answer SHALL match the Ruby gem's payload shapes, with values rounded to four decimal places:
- `choice`: `type=choice`, the winning `choice` label (first-index argmax tie-break on the tempered probabilities), the full `probabilities` map label → probability, `confidence`, and `action.act_probability`
- `score`: `type=score`, `score` = Σ i·p over the tempered probabilities (unrounded, then rounded), a `legend` of level index → description, the per-level `probabilities` map, `confidence`, and `action.act_probability`
- `noul`: `type=noul`, `noul` = tempered probability of the `true` option, `confidence` = max(p, 1−p) computed from the unrounded value, and `action.act_probability`
- Every answer carries `action.act_probability` = first component of softmax over `act_logits`

#### Scenario: Single-option choice is fully decided

- **WHEN** a `choice` question has exactly one option
- **THEN** the option is padded to two markers with the second masked off, the answer's top probability is 1.0, and its confidence is 1.0

#### Scenario: Noul confidence reflects distance from coin flip

- **WHEN** a `noul` answer's probability is below 0.5
- **THEN** its confidence equals 1 − probability (rounded), not the probability itself

### Requirement: Calibration parity

Probabilities SHALL be computed exactly as the Ruby gem does: logits divided by the fitted temperature for their bucket (`qtype:k` with k sizes `2`, `3-5`, `6-10`, `11+`), softmax over only the live options, confidence `1 − H(p)/log k` (1.0 when k < 2), and temperatures clamped to [0.5, 5.0] with non-numeric values replaced by 1.0. A checkpoint whose shipped temperature falls outside the clamp (e.g. 0.1006) SHALL be answered with the clamped value exactly as the gem does.

#### Scenario: Out-of-range temperature is clamped

- **WHEN** the fixture checkpoint ships `choice:11+` temperature 0.1006
- **THEN** answers for an 11+ option choice use temperature 0.5, matching the Ruby reference fixture

#### Scenario: Non-numeric temperature is neutralized

- **WHEN** a temperature bucket value is not a number
- **THEN** that bucket answers with temperature 1.0

### Requirement: Sequence construction parity

Input sequences SHALL be built with the Ruby gem's `build_sequence` semantics: `[CLS] <type> question: <instructions> [SEP]` head, each option as `[MASK] <option text>` capped at 48 tokens, options trimmed to equal shares when they exceed the head budget (minimum 4 tokens each, head at least 8), `[SEP]`, then the state text consuming the remaining budget, then a final `[SEP]`, truncated to `max_len`. `marker_pos` SHALL record each `[MASK]` position; sequences SHALL be padded to at least `min_seq` (8) tokens and `marker_mask` SHALL mark only the live options, padding to at least `min_markers` (2). Any `[MASK]` token in instructions, options, or the state SHALL be replaced by a space as the gem does.

#### Scenario: Long state is right-truncated to budget

- **WHEN** a state text exceeds the remaining sequence budget at `max_len`
- **THEN** the last tokens of the state are truncated and the answer matches the fixture computed under the same budget

#### Scenario: Marker positions follow the template

- **WHEN** a batch is built for a request
- **THEN** each row's `marker_pos` is the position of each option's `[MASK]` in `input_ids`, and masked-out padding markers are not scored

### Requirement: Decision runs do not request the encoder output

The decide path SHALL request only `logits` and `act_logits` from the graph. The graph SHALL NOT be asked to materialize `last_hidden_state` on the decide path.

#### Scenario: Output selection is per run

- **WHEN** a decide request executes
- **THEN** the run returns `logits` and `act_logits` and the encoder state is not materialized (ORT output pruning), exactly as the Ruby gem's runtime behaves