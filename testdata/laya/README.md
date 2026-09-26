# Laya parity fixtures

Vendored from the ruby-laya gem (Apache-2.0): `test/fixtures/tiny/*` of
https://github.com/codenamev/ruby-laya at `laya 0.3.7`.

- `model.onnx` — a 32-hidden-size Laya decision model (encoder + typed head +
  action head), exports of the `convaiinnovations/laya` checkpoints. Inputs:
  `input_ids`, `attention_mask` (int64 `[b, seq]`), `marker_pos` (int64
  `[b, markers]`), `marker_mask` (bool `[b, markers]`), `qtype` (int64 `[b]`).
  Outputs: `logits` `[b, markers]`, `act_logits` `[b, 2]`,
  `last_hidden_state` `[b, seq, dim]`.
- `onnx_config.json` — export provenance: `hidden_size 32`, `min_seq 8`,
  `min_markers 2`.
- `rl_agent_config.json` — token budgets (`max_len 64`, `head_max_len 32`)
  and temperature buckets, including out-of-range (`choice:11+` 0.1006) and
  non-numeric (`bad`) values the calibration clamps.
- `expected.json` — the gem's parity corpus: predict answers for 8 cases
  (per-question `choice`/`score`/`noul` payloads with `action.act_probability`
  and `input_tokens` usage), the applied temperature table, and embedding
  reference data.
- `tokenizer/` — the checkpoint's WordLevel tokenizer (`tokenizer.json` +
  `tokenizer_config.json`; specials `[PAD] 0/[UNK] 1/[CLS] 2/[SEP] 3/[MASK] 4`).