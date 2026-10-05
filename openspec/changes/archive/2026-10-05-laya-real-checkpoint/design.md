# Design

See proposal.md — Why.

## Context

The sandbox runs one `shared-cpu-1x` / `2gb` machine (`website/repl/fly.toml`) with
six retrieval models downloaded onto its volume, plus the `testdata/laya` miniature
shipped in the image. `hfhub.DownloadModel` fetches `model.onnx` / `onnx/model.onnx`
and the supporting files from the repository **root**; `internal/config`'s
`ModelConfig` has no subfolder field. The plate calls every example against model
`laya` (the miniature). The published exports (`codenamev/laya-onnx`) store each
checkpoint under a folder and are fp16 on disk:

| export | params | file | context |
|---|---:|---:|---:|
| `english/` | ModernBERT-large ~403M | 807 MiB | 512 |
| `multilingual/` | mmBERT-base ~308M | 617 MiB | 1024 |
| `typed-decisions/` | ModernBERT-large ~403M | 807 MiB | 1024 |

Their `rl_agent_config.json` carries the real envelope (e.g. `english`:
`max_len 512`, `head_max_len 192`, temperatures `[1.6369, 1.2514, 1.9834]` plus the
`temperature_by_options` table). The graph contract matches `laya.lua` exactly
(`input_ids`, `attention_mask`, `marker_pos`, `marker_mask`, `qtype` → `logits`,
`act_logits`).

## Goals / Non-Goals

**Goals:**
- The typed-question examples answer from a real checkpoint, so their decisions can
  be trusted and the plate's own ticket labels mean something.
- The loops stay realtime — they keep the miniature.
- The download path stays pure Go/HTTP and one-time-onto-the-volume.

**Non-Goals:**
- Changing the loops' model, or making the miniatures' numbers meaningful.
- Per-request model loading, or a general multi-checkpoint router in the server.
- An English/multilingual routing decision at request time (one checkpoint is
  mounted for the demo).

## Decisions

### D1 — Mount the `typed-decisions` export as the demo's real checkpoint

The Inbox set includes a Spanish ticket and needs one checkpoint to cover every
ticket, so the first cut used `multilingual` (smallest, 100+ languages). Measured
against the deployed sandbox, that export is the wrong one: its
`rl_agent_config.json` carries identity temperatures (`[1,1,1]`), an empty
`temperature_by_options`, and `fine_tuned_from_checkpoint: false` — it is the
router export, not the triage one — and it marked a duplicate-charge refund 96%
spam and 99.97% phishing. `english` is fine-tuned but mishandles the Spanish
ticket. `typed-decisions` is fine-tuned for invoice/security/customer-service
workflows, is ModernBERT-large (same ~3.3 GiB footprint as `english` with one
session per path), and is coherent across the whole ticket set — only the
phishing ticket scores spam/phishing true, and the Spanish ticket reads `billing`
with low spam. **Alternative:** mount both `english` and `multilingual` and route
per ticket (more memory and a second entry; rejected for simplicity).

### D2 — The plate routes per example

The typed-question examples call `EMB.EVSHA <real-model> <laya.lua digest> …`; the
loops call `EMB.EVSHA laya <snake|pacman digest> …`. The preset bytes are the same
`laya.lua` for both, so the stamped digest is unchanged; the sandbox preloads
`laya.lua` under both entries. **Why:** the plate's copy already says loops run one
call per episode; the real model is far too slow for a 96-tick episode on one shared
vCPU. **Alternative:** the real model for everything (rejected: an episode would
take tens of seconds); the miniature for everything (the current, broken state).

### D3 — Subfolder download support

`ModelConfig` gains `model_subfolder`; when set, `DownloadModel` resolves the ONNX
(`FindONNX`/`FindQuantizedONNX` over the subfolder's files) and the supporting files
(`<subfolder>/tokenizer.json`, `<subfolder>/tokenizer/tokenizer.json`, …) and writes
them to the model directory under their conventional names. **Why:** the export's
layout is not negotiable and mirroring it would be a second host to keep in sync.
**Alternative:** a wrapper repo with a root layout (a maintained copy; rejected).

### D4 — Memory is paid by the machine, or by quantization

The real export is ~617 MiB fp16 and ORT materializes fp32, so the mounting machine
needs ~1.2 GiB + activations for it. The 2 GB sandbox is already near its ceiling, so
the change lands with one of:

- **grow the machine** (`shared-cpu-2x` / 4 GB, or `performance-1x`) and load the
  fp16 export — exact, simplest, ~2× the sandbox cost; or
- **produce and host an int8 export** and keep 2 GB — cheaper per month, but adds a
  quantization/build step and a fidelity check against the fp16 answers (the HF card
  documents fp16 as exact; int8 drift must be measured, not assumed).

**Recommendation:** grow the machine for the first landing (measure RSS, keep the
fp16 answers exact), and add the int8 export as a follow-up if the cost matters.
**Alternative:** a second Fly app just for the checkpoint (keeps the sandbox's cost
and isolation, but adds a second deployment and cross-app routing in the bridge).

**Outcome (measured on the deployed sandbox).** The estimate above was low. On Linux,
the batch-determinism probe fails for these exports, so each model's pool degrades
to `workers` auto-tuned sessions and the checkpoint is loaded once per worker; a
2 GB machine OOM-killed `emb` at the `laya-real` load. Pinning `workers: 1` in
`sandbox.yaml` (the bridge serializes upstream work anyway) restores one embedding
session plus the scripted session, and the fully loaded set is ~3.7 GiB steady.
The sandbox now runs `shared-cpu-4x` / 8gb. The int8 export remains the cheaper
follow-up.

### D5 — The entry carries the checkpoint's own envelope

The real entry's `config:` is copied from its `rl_agent_config.json` /
`onnx_config.json` (`max_len`, `head_max_len`, `min_seq`, `min_markers`, the
temperature table), not the miniature's rounded values. **Why:** calibration is part
of the answer; the round-trip the preset ports is exact only with the real envelope.

### D6 — The plate says which model answered

The per-example label distinguishes "the shipped miniature" (loops) from the real
checkpoint (typed questions), and the stand-in caveat applies only to the loops. The
exact-commands disclosure keeps showing the invocation that produced the reply.

## Risks / Trade-offs

- **Tokenizer compatibility** → the `multilingual` tokenizer is a BPE/WordPiece
  export; the host's `SpecialTokenIDs` / `EncodePlain` were validated on a WordLevel
  fixture, so the real tokenizer must be exercised before merge.
- **First-boot download** (~617 MiB) may exceed the ready-check grace period →
  pre-seed the volume, or accept the check's warning on the first boot.
- **Inference speed** — a ~300M model on one shared vCPU answers the Inbox in
  hundreds of milliseconds; the bridge's 30 s deadline and the reply cache cover it.
- **Memory measurement** → the fp16/fp32 RSS must be measured before widening the VM
  or trusting 2 GB.
- **Quantization drift** (if D4's second path) → an int8 export must be validated
  against the fp16 answers before it ships.

## Migration Plan

Additive: the miniature stays; the real entry is new. Rollback is removing the real
entry and reverting the plate's routing. The sandbox is hand-deployed, so the change
lands with `just sandbox-deploy` and a first-boot download; the volume keeps the
weights afterwards.
