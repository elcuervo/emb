# Proposal

## Why

Validated against the running sandbox: the decision plate's Inbox and Quickstart
examples answer with the shipped **32-hidden random-weight miniature**, so their
"decisions" are coin flips. Across the eight Inbox tickets every answer lands near
even (category 16.7% of a 6-way choice, `is_spam` ~50.2%, `needs_reply` ~50.2%),
and the decisions contradict the labels the plate itself prints — 8/8 tickets are
marked `is_spam true`, and both phishing/compromise tickets come back
`is_phishing false`. A plate whose subject is inbound triage cannot rest on a model
that answers nothing; the figure is faithful to the reply, but the reply means
nothing. (The full table: the ticket, the demo's claim, and the reply's decision.)

## What Changes

- Serve the plate's **typed-question examples** (Inbox, Quickstart) from a **real
  published Laya checkpoint** — an export of `codenamev/laya-onnx` — mounted as its
  own model entry. The **Snake and Pac-Man loops keep the shipped miniature**, so
  their one-call episodes stay realtime on the sandbox's single shared vCPU.
- Teach the model downloader to fetch a checkpoint from a **repository subfolder**:
  the Laya export publishes each checkpoint under `english/`, `multilingual/`,
  `typed-decisions/`, not at the repository root.
- Carry the real checkpoint's own envelope (`max_len`, `head_max_len`, the
  temperature table) into the model entry, instead of the miniature's rounded values.
- Size the sandbox for the real weights or replace them with a quantized export:
  the export is ~617 MiB fp16 (multilingual) to ~807 MiB (english) and ONNX Runtime
  materializes fp32, which the current 2 GB machine cannot hold beside its six
  retrieval models.
- Route the plate's calls per example (typed questions → the real entry, loops →
  the miniature) and label which examples answer with a real checkpoint and which
  are the stand-in.

No **BREAKING** changes.

## Capabilities

### New Capabilities

- None.

### Modified Capabilities

- `huggingface-model-download`: a model may be downloaded from a named repository
  subfolder.
- `sandbox-service`: the showcase serves a real decision checkpoint for its
  typed-question examples, alongside the shipped miniature for its loops.
- `embedding-demos`: the stand-in rule is scoped to the loops; a typed-question
  example answered by a real checkpoint is not presented as a substitute.

## Impact

- **Server**: `internal/config` gains `model_subfolder`; `internal/hfhub` resolves
  the ONNX and supporting files under it; tests for both.
- **Sandbox**: `website/repl/sandbox.yaml` gains the real entry and its envelope and
  preloads `laya.lua` under it; `website/repl/fly.toml` memory (or a quantized
  export); `website/repl/Dockerfile` unchanged unless the checkpoint ships in the
  image instead of the volume.
- **Site**: `website/demos/laya.html` routes the typed-question calls to the real
  entry and states per example which model answered; `website/tools/stamp-presets.py`
  keeps mapping the preset to its model.
- **Docs**: `website/docs/index.html` §10 and `README.md` name the real checkpoint
  and the two-model sandbox.
- **Decision, settled during implementation**: the mounted checkpoint is
  `typed-decisions` — `multilingual` covers the Spanish ticket but ships
  uncalibrated and marked legitimate mail as spam/phishing, while `english`
  mishandles the Spanish ticket; the memory is paid for by widening the machine
  to 8 GB.
