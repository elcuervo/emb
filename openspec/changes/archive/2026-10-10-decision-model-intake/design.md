# Design

## Context

See `proposal.md` — Why. Two layers are involved:

```
internal/hfhub/hfhub.go         network + artifact selection (ListFiles → FindONNX/FindQuantizedONNX → Download)
internal/registry/registry.go   load-time resolution (resolveQuantize → preferredQuantizedPath) and labelling
internal/config/config.go       quantize: auto | on | off
```

Measured evidence (live repositories, 2026-10-10):

| Fact | Value |
|---|---|
| `models/laya-typed/model.onnx` (local fp32) | 846 MB, opset 20, 5 inputs, 3 outputs (`logits [B,K]`, `act_logits [B,2]`, `last_hidden_state [B,seq,1024]`) |
| `yehor-oleksiuk/laya-typed-decisions-onnx` `model_int8.onnx` | 506 MB, opset 21 + `com.microsoft`, same 5 inputs, 2 outputs (`last_hidden_state` omitted), `tokenizer.json` present |
| `onnx-community/laya-typed-decisions-ONNX` | `onnx/model.onnx` 4.4 MB + `onnx/model.onnx_data` 1685 MB (+ an fp16 pair, 842 MB data) |
| `scripts/laya.lua` output request | `emb.run(batch, { outputs = { "logits", "act_logits" } })` — never `last_hidden_state` |
| Existing quantized names | `model_quantized.onnx`, `onnx/model_quantized.onnx`, `onnx/quantized/model.onnx` |
| Existing int8 label rule | base name contains `quantized` or `int8` (already correct) |
| `onnx-community/laya-typed-decisions-ONNX` `onnx/model.onnx` | 4.4 MB graph, opset 18, same five inputs, two outputs, weights in `onnx/model.onnx_data` (1685 MB) |

### Candidate probe (task 1.2, recorded 2026-10-10)

Every row read from the published files, not from a model card. Licences checked through the
Hub API: all apache-2.0.

| Candidate | Graph | Contract vs the five laya inputs | Verdict |
|---|---|---|---|
| `onnx-community/laya-typed-decisions-ONNX` | opset 18, `onnx/model.onnx` 4.4 MB + `onnx/model.onnx_data`, fp16 pair alongside | identical (`input_ids`, `attention_mask`, `marker_pos`, `marker_mask`, `qtype` → `logits [B,K]`, `act_logits [B,2]`) | **mountable by this change alone** — `scripts/laya.lua` fits as-is; only the sidecar download blocks it (task 3) |
| `ParallaxOpen/Vela-Decision-170M` | no ONNX; `VelaDecisionForOptionScoring` behind `auto_map` custom code, plus a separate `act_head.pt` | differs: `marker_pos` + `option_slot`, no `marker_mask`, no `qtype`; act head pooled from a late position | not drop-in — needs an ONNX export of custom code and its own preset; own change |
| `MaziyarPanahi/ModernJEV-Decide-Preview` | no ONNX; plain `ModernBertForSequenceClassification`, hidden 768, 22 layers, one label | a pair classifier (Family C), not marker-scored | not drop-in — needs a pair-classifier preset; the export itself is standard; own change |
| `Lukitaduena/dinah-0` | ONNX int8/8bit published | unknown | gated (hub API 401) — cannot inspect; re-check before relying on it |

Verified end to end for the int8 artifact (task 1.1): it loads under `nix develop` and answers —
`department=billing` 0.784, `refund` 0.671, `urgency` score 1.629, 815 ms, `input_tokens` 257.

Head to head against the checkpoint the site mounts, one model per process, five uncacheable
payloads each (recorded 2026-10-10):

| | fp16 (site's checkpoint) | int8 export |
|---|---|---|
| graph on disk | 846075712 B (807 MiB) | 505942535 B (483 MiB) |
| process RSS, 1 worker | 5449 MB | 2230 MB |
| warm call, median of four | 321 ms | 392 ms |
| input tokens | 182 | 182 |

The weights are 1.67x smaller and the resident footprint 2.4x smaller, for ~22% more latency per
decision (dynamic quantization pays a per-batch quantize/dequantize cost its narrower weights do
not earn back at these shapes). Quality is on par — see the parity numbers above. That trade is
why the sandbox adopting int8 stays a separate change: at 5.3 GB, `laya-real` is the largest
single consumer on the 8 GB machine, so the memory is worth more there than the milliseconds.

### Website verification (task 5.2, recorded 2026-10-10)

`just website-dev` on local ports (8090 site / 8091 bridge / 6390 emb, so a stack already on the
default ports keeps them). The dev server rewrote the plate's sandbox origin to
`127.0.0.1:8091`, and the served page carried the same three preset digests the bridge reports.
All four plate examples answered through the bridge contract the page itself uses
(`POST /api/exec`): Quickstart `billing` 0.7847 / `refund` 0.668 / `urgency` 1.6261 (257 tokens,
1009.9 ms), the Inbox phishing ticket `security` 0.7279 / `is_phishing` 0.5535 / `needs_reply`
0.5187, and Snake and Pac-Man 8 frames each from the miniature (Pac-Man's planner vetoed 3 ghost
moves). Driven in the browser, the Quickstart tab rendered `billing 78.5% / critical deadline
1.63 / true 66.8%` — the same numbers to the digit the bridge had returned, so the page was
served by this stack, not the published one. `laya-real` resolved `models/laya-typed/model.onnx`
(846075712 bytes, unchanged) and `laya` the 778051-byte miniature.

Constraints: pure-Go HTTP downloads (no Python), one selected graph per model directory, ORT
resolves `<graph>.onnx_data` beside the graph on its own.

Direct consumer: the preview site's sandbox mounts `laya-real` as `model_repo: codenamev/laya-onnx`
+ `model_subfolder: typed-decisions` with `quantize` unset (`auto`) and its own volume path
(`website/repl/sandbox.yaml`), i.e. straight through `FindQuantizedONNX` → `DownloadModel` —
the two functions this change edits. `codenamev/laya-onnx` publishes only `model.onnx`,
`onnx_config.json`, `rl_agent_config.json` and the tokenizer per subfolder: no quantized member,
no int8 member, no sidecar. So the site's artifact must not move, and that is checkable.

## Goals / Non-Goals

**Goals:**
- A third-party int8 decision export mounts with `quantize: auto`, from a repo name or a local path.
- A split export (graph + `.onnx_data`) downloads whole and loads.
- The reduced-output int8 graph is a supported mount, proven by an opt-in verification run.
- The next decision-model candidate is chosen from a recorded contract dump, not a guess.

**Non-Goals:**
- Any second decision preset, or a `clef`/KV-cache family path.
- fp16 artifacts, wildcard int8 matching, or converting/quantizing weights inside emb.
- Changing `scripts/laya.lua`, the wire contract, or any command.

## Decisions

**1. Extend the quantized name lists; add no new ordering logic.**
`FindQuantizedONNX` and `preferredQuantizedPath` gain `model_int8.onnx` and
`onnx/model_int8.onnx`, after the `model_quantized.onnx` family. The
"int8 outranks sibling fp32" requirement is then satisfied by selection, not by a new
comparator: `DownloadModel` already tries the quantized finder before `FindONNX`, and
`resolveQuantize` already points at the preferred path when it exists.
*Alternative rejected:* marking quantized-vs-fp32 preference inside `FindONNX`'s alphabetical
sort — it would make fp32 resolution order-dependent too, and `FindONNX` is used when
quantization is off.
*Alternative deferred:* wildcard matching (`*int8*.onnx`, which would also cover
`falco_modernbert_int8_dynamic.onnx`). Left out to keep the change small; a wildcard can pick an
unrelated export in a multi-graph repo. `shortcut: fixed int8 names only, add a wildcard when a
widely used repo ships an int8 graph under a third name.`

**2. Sidecars are selected from the file list, not probed.**
`ListFiles` already returns every path in the repository, so sidecars are chosen by the rule
`<selected graph path> + "_data"` (and the same rule with the fp16 sibling) — no extra API call,
no HEAD probe.
*Alternative rejected:* downloading every `*.onnx_data` in the repository — it would pull an
unselected 842 MB fp16 sidecar.

**3. The sidecar transfer is independent of the graph's existence check, and a failed
transfer rolls back a freshly downloaded graph.**
`Download` early-returns when its destination exists, so the sidecar is fetched on its own
existence check. The failure path matters as much: if the sidecar transfer fails after the
graph was published, the graph is removed again (only when this call is the one that
downloaded it, so a file the operator placed by hand is never deleted). Otherwise the next
boot would see the graph present, skip the download, and fail to load forever.
*Alternative rejected:* a boot-time repair that lists the repository whenever a graph is
cached. It would put a network call on the boot path of every `model_repo` entry, with an
unbounded stall when the Hub is unreachable — a bad trade for a cache that is loud when
broken (the load error names the missing `_data` file) and fixed by deleting one file, which
is what `docs/configuration.md` now says.

**4. Reduced-output graphs need verification, not code.**
`selectOutputTensor` already falls back by rank and the preset names its own outputs, so no
loader change is required. The risk is that the fallback is non-deterministic for a scripted
graph (map iteration picks `logits` or `act_logits` first) — harmless today because scripts
never read `cfg.OutputTensor`, and recorded in `docs/` rather than fixed here.

**5. Verification of the real artifact is opt-in, mirroring `verify-embeddings`.**
The 506 MB download does not belong in `just test`. A verification target downloads the int8
artifact and answers the vendored corpus with both artifacts, asserting the parity tolerance in
the `int8-weight-quantization` delta.
*Alternative rejected:* committing a second miniature fixture — it would prove the code path but
not the artifact that motivated the change.

**6. Candidate selection is a spike, with a fixed checklist.**
For each candidate: ONNX input/output names, opset and quantization domains, sidecar layout,
whether the graph needs any input the preset does not supply, and licence. Recorded in Context
above, before any preset is written. Result: only `onnx-community/laya-typed-decisions-ONNX` fits
the existing preset, and this change already makes it mountable through the sidecar work, so no
second preset is written here; Vela and ModernJEV-Decide get their own changes.

**7. The website is validated on the composed stack, and the sandbox config is not touched.**
`just website-dev` runs the site, the bridge and a local emb together against
`website/repl/sandbox.yaml`, which is the only way to exercise local intake end to end; the
published tree (`just website`) loads the module from `cli.emb.is` and cannot. The sandbox keeps
mounting the miniature plus the published `typed-decisions` checkpoint — moving a live face of
the product onto int8 is a separate change with its own copy and deploy, and AGENTS.md requires a
hand `just sandbox-deploy` for sandbox edits.
*Alternative rejected:* asserting the site in a spec delta — the demos' requirement ("the demos run
against the models the surface serving them reports") already covers what the plate must state,
and no requirement changes. Only the download capability gains a requirement, pinning the
resolution the site depends on.

## Risks / Trade-offs

- **Quantization changes a decision.** → Measured (task 4.2, `just verify-laya-int8`): over the
  site's own payloads (19 payloads, 151 probabilities, choice/noul/score), the published int8
  export matches the checkpoint the site mounts on every winning option, with a worst
  probability delta of **0.0158** against the 0.05 tolerance. Tightening `EMB_LAYA_INT8_TOL` to
  0.01 fails on that same answer, so the assertion is real rather than decorative.
- **`just website-ink` fails on the decision plate (pre-existing, out of scope).** → Measured
  while validating the site: `demos/laya.html` reports text ink outside the viewport at 2 of 24
  widths (834, 320) through the same dev server where `demos/batch.html` and `docs` pass all 24.
  No file under `website/` changed here (`git status --short website/` is empty) and
  `just website-presets-check` is green, so the plate's own layout is the subject; it needs its
  own change rather than a widening of this one.
- **The nix ORT build rejects opset 21 or the `com.microsoft` int8 ops.** → Checked first (task
  1.1) and it does not: the 506 MB int8 graph loaded and answered under `nix develop`
  (`opset 21` + `com.microsoft`, 815 ms). Two observations from that run: the reduced graph makes
  `InferDim` report `dim: -1`, so a scripted model's `dim` must not be trusted (its `output_tensor`
  auto-picks a rank-2 output too), and `act_probability` came back exactly `1.0` — independently
  reproducing the ParallaxOpen card's finding that laya's act head carries no signal.
- **A cached directory from before this change stays broken.** → Decision 3 (independent
  sidecar check).
- **Selection by name is brittle when a repo renames its artifact.** → The failure is loud
  (`quantize: on` errors; `auto` logs a fallback warning) and is covered by a task to log the
  resolved filename at load, so the next report is diagnosable.
- **Bigger transfers for split repos (1.7 GB).** → Only the selected graph's sidecars are
  fetched; fp16 is not in any candidate list.
- **Third-party repositories churn, and some are non-commercial licences.** → Docs pin the
  verification target to a specific revision; emb ships no third-party weights.
- **Non-deterministic auto-detected `output_tensor` for scripted graphs (pre-existing).** →
  Documented; out of scope to change.
- **The live site mounts a Hub repo through the exact path this change edits.** → Pinned by a
  test over the repository's file list (task 5.4) and validated on the composed stack (task 5.2),
  including that the served `laya-real` artifact is still `typed-decisions/model.onnx`.

## Migration Plan

Additive: no config, wire, or on-disk layout change, and no removal. A model directory that
already loads keeps loading. Rollback is reverting the change; a directory that received
sidecars keeps working with the previous binary because the extra file is ignored, while a
directory that mounted an int8-named artifact would need its `onnx:` path set explicitly.
