# Configuration

Server settings live in a YAML file (`-config config.yaml`) or inline flags.

- [Full example](#full-example)
- [Model options](#model-options)
- [Image models and preprocessing parity](#image-models-and-preprocessing-parity)
- [Preloading scripts](#preloading-scripts)
- [Batching](#batching)
- [Caching](#caching)
- [Persistent cache snapshots](#persistent-cache-snapshots)

## Full example

```yaml
listen: ":6379"
cpu_budget: auto              # process-local ORT budget; use off for legacy sizing
# cpu_reserve: 1               # auto budget only; default reserve is 2/1/0 by CPU tier
# calibrate: true              # briefly benchmark automatic model layouts at load

# password: "hunter2"
# tls_cert: /etc/emb/cert.pem
# tls_key:  /etc/emb/key.pem
# cache: "auto"   # or "1GB", "256MB", "25%". Empty = disabled
# cache_file: /var/lib/emb/cache.embcache
# cache_load: true
# cache_save: 5m
# cache_save_on_shutdown: true
# cache_restore_limit: auto
# cache_restore_reserve: 10%
# cache_save_rate_limit: 100MB/s
# idle_timeout: 15m, max_connections: 100, max_concurrent_requests: 32
# max_command_bytes: 64MB       # max buffered bytes per command (0 = unlimited)
# max_images: 4096              # images per EMB.IMG/EMB.IMGMULTI (0 = unlimited)
# max_image_bytes: 32MB         # per-image byte cap (0 = unlimited)
# max_image_pixels: 33554432    # decoded pixels per image, header-checked (0 = unlimited)

# telemetry:                     # OTLP/HTTP metrics export; disabled unless an endpoint is set
#   endpoint: http://localhost:4318
#   interval: 30s
#   service_name: emb
#   host: emb-1
#   headers:
#     - "dd-api-key=..."
#   attributes:
#     deployment.environment: prod

models:
  minilm:
    onnx: ./models/minilm/model.onnx

  # Pre-pooled siglip2 text encoder (int8 used in production). The graph
  # exports its final embedding as "text_embeds" (768-dim); if you configure
  # an output tensor name that is not in the graph, the server falls back to
  # the detected output and logs which name it auto-selected.
  siglip2:
    onnx: ./models/siglip2/text_model_int8.onnx
    tokenizer: ./models/siglip2/tokenizer.json
    output_tensor: text_embeds
    pooling: none
    normalize: true
    dim: 768
    max_length: 64
    pad_output: true   # graph has a fixed [batch, 64] input; pad to max_length

  # Custom e5 export with pooling + output layers baked into the graph (2D
  # pooled output, already normalized), served with no server-side arithmetic:
  # pooling: none + normalize: false is a pure buffer export.
  e5:
    onnx: ./models/e5/model.onnx
    tokenizer: ./models/e5/tokenizer.json
    output_tensor: pooled_sentence_embeddings_debiased_normalized
    pooling: none
    normalize: false

  # WARNING: the stock HuggingFace export intfloat/e5-small-v2 (and similar)
  # outputs a 3D last_hidden_state. It must NOT be configured with
  # pooling: none — that would slice a 3D buffer (correct only at batch=1).
  # Use pooling: mean (auto-detected) or a pre-pooled 2D export instead.

  # Dual-encoder vision export (CLIP/SigLIP): text EMB and image EMB.IMG share
  # one embedding space. The image: block enables EMB.IMG; omitted fields are
  # auto-detected from the ONNX graph and preprocessor_config.json.
  clip:
    onnx: ./models/clip/model.onnx
    tokenizer: ./models/clip/tokenizer.json
    output_tensor: text_embeds        # text branch output
    pooling: none
    normalize: true
    dim: 512
    image:
      input: pixel_values             # image branch input tensor
      output: image_embeds            # image branch output (dim must match dim)
      size: 224
      crop: center                    # none | center
      resample: bicubic               # nearest | bilinear | bicubic | lanczos
      rescale: 0.00392156862745098    # 1/255
      mean: [0.48145466, 0.4578275, 0.40821073]
      std:  [0.26862954, 0.26130258, 0.27577711]
    image_preload: true
```

## Model options

| Field | Default | Description |
|-------|---------|-------------|
| `weight` | `1` | Relative fair-share weight for admission turns when models contend for the process inference budget. It is not a reservation or a cap: an idle model's share is borrowable and does not limit another model's layout |
| `onnx` | — | Path to ONNX model file |
| `tokenizer` | `<model-dir>/tokenizer.json` | Path to HuggingFace tokenizer JSON |
| `model_repo` | — | HuggingFace repo (auto-downloads ONNX + tokenizer) |
| `dim` | auto-detected | Embedding dimension |
| `max_length` | auto-detected (or 512) | Max token sequence length |
| `pooling` | auto-detected | `mean` (3D output), `cls` (first token) or `none` (2D pre-pooled) |
| `normalize` | `false` | L2-normalize the output |
| `quantize` | `auto` | Weight precision. `auto` prefers a pre-quantized artifact beside `onnx` when the directory has one and falls back to fp32 otherwise; `on` requires one (load fails without it); `off` keeps fp32. Accepted names, in priority order: `onnx/model_quantized.onnx`, `model_quantized.onnx`, `onnx/quantized/model.onnx`, `model_int8.onnx`, `onnx/model_int8.onnx` — the same names a `model_repo` download resolves. Boot names the resolved file, so a fallback to fp32 is visible rather than silent, and `EMB.INFO` reports `quantization` and `model_bytes` |
| `output_tensor` | auto-detected | ONNX output tensor name |
| `preload` | `false` | Load model at startup instead of on first request |
| `pad_output` | `false` | Pad sequences to `max_length` with trailing zeros (compatibility with legacy implementations that don't pass attention mask) |
| `workers` | calibrated when unset with `cpu_budget: auto`; legacy auto-tuning with `off` | Embedding sessions for the unbatched path; explicit values remain honored |
| `script_workers` | `1` with `cpu_budget: auto`; legacy auto-tuning with `off` | Named-tensor sessions for scripted models (`EMB.EVAL`/`EMB.EVSHA`); explicit values remain honored |
| `script_callers_per_session` | `4` | Concurrent evaluations allowed per scripted session (the analyzed sweet spot). ORT sessions are safe for concurrent `Run`, so a lone scripted model is not serialized behind one session. `0`/absent = 4; set `1` to serialize |
| `allow_spinning` | off for embedding/image under automatic budgeting | Maps to ORT `session.intra_op.allow_spinning`; explicit values remain honored |
| `autotune` | `auto` | `auto`/`callers` enable the runtime concurrency controller; `off` fixes concurrency at the configured value. Applies to scripted sessions |
| `capacity` | `auto` | Creation-time layout: `auto` (derived layout plus runtime adaptation), `latency` (spinning on, one caller per session), or `throughput` (spinning off, shared sessions) |
| `intra_op_threads` | calibrated when unset with `cpu_budget: auto`; legacy default with `off` | ONNX intra-op threads per session. An explicit value is honored; runs wider than the process budget are admitted alone |
| `scripts` | `[]` | List of file paths to Lua scripts to preload at boot. Relative paths resolve against the config file's directory; absolute paths are used as-is. Invalid scripts (bad syntax, missing file, oversized) fail startup |
| `image` | — | Image preprocessing block (enables `EMB.IMG`); see [Image models](#image-models-and-preprocessing-parity) |
| `image_preload` | `false` | Warm the image named-session pool at startup instead of on first `EMB.IMG` |
| `batching` | `{timeout: 1, max_batch: 32, max_batch_tokens: 16384}` | Smart batching settings. **Enabled by default** (1 ms window) for every model; set `timeout: 0` to use the worker pool. With batching on, `tokenize_workers` defaults to `min(4, cores)` and the token budget auto-applies. Batching is batch-determinism gated automatically (no flag): dynamic-quantized int8 graphs degrade to the worker pool at load (see *Batch determinism*) |

### Quantized weights

`quantize` decides whether a pre-quantized sibling wins over the file `onnx` names. A
checkpoint that publishes only an int8 file is mounted by naming that file; the graph's
inputs and outputs are read from the file itself, so a reduced export (one that omits
`last_hidden_state`) mounts the same way.

```yaml
models:
  laya:
    onnx: ./models/laya-typed/model.onnx       # or ./models/laya-typed/model_int8.onnx
    tokenizer: ./models/laya-typed/tokenizer.json
    quantize: auto                              # loads model_int8.onnx when it is beside it
```

With `auto`, drop `model_int8.onnx` beside the fp32 file and the next boot loads it and
says which file it resolved. Prefer an explicit path when a directory holds several
checkpoints.

### Weight memory on the CPU execution provider

Resident memory is set by what the execution provider materializes, not by the file size,
and it grows on the first inference rather than at load. Measured with the decision
checkpoint (ModernBERT-large) on this machine, one worker, one request:

| weights on disk | RSS after load | RSS after the first call |
|---|---|---|
| 807 MiB (fp16 export) | 2725 MB | 4897 MB |
| 483 MiB (int8 export) | 1235 MB | 2280 MB |

`intra_op_threads` 1 and 4 land within 40 MB of each other, so this is not arena
spawning: the provider expands the weights to fp32 to compute with them (and the int8
path keeps fp32 copies of what it dequantizes). Size a host from the *resident* figure —
roughly 3-6x the weight file — and prefer an int8 export where memory binds rather than
throughput: the table above is the same model answering the same request.

A scripted model's `dim` and `output_tensor` are auto-detected from its graph and then
unused — the preset names its own inputs and outputs. A reduced export with no rank-3 output
therefore reports `dim: -1` and may pick either of its rank-2 outputs (`logits` /
`act_logits`), which does not change an `EMB.EVAL`/`EMB.EVSHA` answer.

### Split exports (graph + external weights)

Most exports keep their weights inside the graph file. Some publish a small graph and put
the weights beside it, as `onnx-community` exports do:

```
onnx/model.onnx        4.4 MB    the graph
onnx/model.onnx_data  1685 MB    the weights ONNX Runtime reads beside it
```

The downloader fetches both and writes them into the model directory under the same base
names, so the graph finds its weights where it expects them; a repository whose graph holds
its weights inline gains no extra transfer. A sidecar that exists but fails to transfer
fails the download and removes the just-downloaded graph, so no half-published export sits
in the cache — a cache left by an earlier version can be repaired by deleting the graph
(the load error names the missing `_data` file), and the next boot re-fetches the pair.

## Thread budget

`cpu_budget` is process-local and defaults to `auto`. The server keeps the
fractional cgroup quota separate from integer capacity and Go scheduler
parallelism. The quota is resolved once, from `/proc/self/mountinfo` and
`/proc/self/cgroup`: cgroup v2 and v1 (combined `cpu,cpuacct` and hybrid)
controller mounts, remapped mount roots, and every visible ancestor up to the
mount point are read, and the tightest positive quota wins; affinity or a lower
`GOMAXPROCS` that is tighter still bounds it. If nothing resolves (no cgroup
mount, unreadable or malformed files, an unlimited hierarchy) the allowance
falls back to scheduler/affinity and the startup log adds
`CPU allowance fallback: <reason>` after the `CPU inference budget:` line, whose
fields are `allowance` (fractional CPUs), `source` (the deciding file or
`scheduler/affinity`), `scheduler`, `affinity`, `capacity`, `reserve` and
`budget`. Sub-one-CPU quotas keep their fraction for saturation accounting (see
[performance validation](./performance-validation.md)); admission capacity
floors the allowance (minimum one), then
reserves two CPUs at 8+, one at 4–7, and none below four. On an 8-CPU task the
initial inference budget is six. `cpu_reserve` overrides that reserve when the
budget is `auto`; it must be smaller than detected capacity. A numeric
`cpu_budget` is an explicit final budget and cannot be combined with
`cpu_reserve`. A budget above detected capacity starts with a warning.

One admission controller covers embedding, image, script and calibration
ONNX runs. Each model has its own FIFO queue; free tokens go to whichever
model's head fits, with contended turns split by `weight` (default 1) and no
idle reservation, so a busy model can use the whole budget while its peers are
idle. A wide head that is passed over for a full round of the active models is
served next, so narrow traffic cannot starve it. Explicit over-budget runs
execute alone. A scripted session is charged its intra-op width once while any
caller runs on it, plus one token for each further concurrent caller on that
session (`script_callers_per_session` bounds the callers; the charge is never
the width divided by the cap). Admission is a bound on admitted work, not an OS
CPU limit. The startup log's `CPU plan` line reports each model's `weight`,
its fair `share`, and its `intra_op_threads` (the whole budget until calibration
or an explicit value changes it). Automatic embedding and
image sessions disable ORT spinning. Set `cpu_budget: off` to restore legacy
session sizing and spinning behavior; this does not roll back the binary.

### Startup calibration

`calibrate` defaults to `true` and applies only when the CPU policy is on
(`cpu_budget` is not `off`).
Defaults require no model-specific tuning. Precedence, highest first:

1. an explicit `workers` / `intra_op_threads` fixes that dimension and is never
   replaced by calibration (the other dimension is still calibrated);
2. an explicit numeric `cpu_budget` sets both the admission budget and the
   candidate ceiling;
3. `cpu_budget: auto` derives capacity and reserve;
4. `calibrate: false` skips the trials and selects a deterministic,
   process-capable layout: one session whose width is the whole process budget
   (`budget / workers` wide when `workers` is set), with explicit values kept —
   not the model's share — while admission stays on. It is not `cpu_budget:
   off`: admission, the memory check below and the CPU budget still apply, and
   the startup `CPU layout` line reports `source=default
   reason=calibration_disabled`;
5. `cpu_budget: off` restores legacy sizing, spinning, and shared scripted
   sessions, and bypasses calibration and admission entirely.

After the batch-determinism probe, each embedding model builds representative
short, typical, and long inputs from its own tokenizer (never user text),
runs one warmup pass, then measures deduplicated width × session candidates
that fit the **process** CPU budget and the process memory headroom (below). The
ceiling is the whole budget, not the model's weighted share, so an idle
configured peer never limits the model being calibrated. A
batching model's trial serves tiled multi-row runs (a representative batch, not
a single row), because width that barely helps one row is worth several threads
for a coalesced batch; the batch shrinks with the fixture's length so one long
run cannot spend the whole deadline. Each candidate is measured twice and keeps
its best score. Candidates whose output differs from the baseline, that report
no CPU accounting, or whose p95 exceeds twice the baseline p95 are rejected;
the highest measured rows per wall-clock second wins, and inside a 5%
run-to-run noise band the **wider** layout wins when the model batches (the
trial understates wide threads) and the lower-resource layout wins otherwise.
An explicit dimension, or a budget with only one candidate, skips the trials.

**Startup cost.** Trials are bounded by a 4-second deadline per model and at
most 12 candidates. The deadline is predicted between candidates from the last
candidate's **total** cost — from before its sessions are created until they are
closed, so slow session construction counts — while candidates are still scored
on their timed inference only. A slow model keeps its best validated layout
instead of overrunning; the deadline is soft around a native call that cannot be
interrupted. Calibration runs when the model loads: at startup for
`preload: true`, otherwise on that model's **first request** — so a lazily
loaded model's first request pays both the model load and up to four seconds of
trials.

**Memory headroom.** Before sessions are created, the server reserves memory
process-wide: the container memory limit (less a 10% margin) minus current RSS,
the cache allowance, one session for every other configured model that is not
resident yet, and what concurrent loads have already reserved. A session is
estimated as the weights file plus any `_data`/`.data` sidecar plus 20%.
Reservations are serialized, so two models loading at once cannot claim the same
headroom. If a larger automatic candidate does not fit, the largest one that
does is used and the layout is reported with `limit=memory`. An explicit
`workers` that does not fit fails the load with an error naming the model rather
than allocating past the limit. Estimates are not measurements: RSS supplies the
measured part, and layouts are chosen at load time.

**Outcome.** The startup log's `CPU layout` line reports `workers`,
`intra_op_threads`, `source` (`calibrated`, `fallback` or `default`), `reason`,
`limit` (`memory` or empty), `ceiling` (the process budget the candidates were
bounded by) and `elapsed` (total calibration time). The same outcome is kept
immutably per model.

**Fallback.** Invalid fixtures, a failed baseline, unavailable CPU accounting,
an expired deadline, or an unusable tokenizer keep the best validated candidate,
or select one session × one thread (or the explicit dimensions) and log a reason
such as `baseline_failed`, `cpu_accounting_unavailable`, `calibration_deadline`,
`candidate_failed`, `p95_regression`, `invalid_limits`, or `invalid_fixture`.
A batch-determinism probe failure turns batching off *before* calibration, so a
degraded model is calibrated for single-row inference and cannot keep
batch-sized worker defaults.

**What it cannot guarantee.** Calibration is a measured startup choice for this
process on this host. It does not change a running layout (restart to re-tune),
does not create CPU isolation, does not tune multiple models jointly, and does
not account for inter-op threads, tokenization, Go code, or sidecars. The
admission budget bounds admitted intra-op work, not physical CPU use. A busy or
noisy host can select a suboptimal layout; `calibrate: false` or explicit
values are the deterministic escape hatch.

The budget bounds admitted intra-op work, not all process CPU use or physical
cores. Tokenization, inter-op work, Go code, and sidecars remain outside that
bound. A reserve is headroom, not CPU isolation. Session layouts are selected
at startup; changing limits requires a restart.

Legacy layout measurements on the reference host (10 cores, GLiNER2 int8,
36-text corpus, no reply cache) at concurrency 8, before the process budget:

| workers × threads | serial p50 | c=4 p50 | req/s | p99 |
|---|---|---|---|---|
| 8 × 1 | 38 ms | 39 ms | 184 | 72 ms |
| 4 × 2 | 23.6 ms | 26.7 ms | 143 | 103 ms |
| 2 × 4 | 16 ms | 38 ms | 98 | 154 ms |
| 1 × 8 | 13–17 ms | 58 ms | 63 | 268 ms |
| 4 × default (8) | 29 ms | – | 24 | 865 ms |

`8×1` gives the best tail and throughput; `4×2` trades some of that for a
better serial latency. Pick from the production `dispatch_wait_us` and `run_us`
counters (see [operations](./operations.md)), not from the laptop numbers
above.

## Shared scripted sessions and spinning

With `cpu_budget: off`, each scripted session is shared by `script_callers_per_session`
(4) evaluations, and spinning is off while a session is shared. A lone scripted
model therefore does not serialize behind one session: measured on the
reference host, one session × 8 threads with 4 callers and spinning off gives
146 req/s where the old `cores−2`-per-session default gave 24. Set
`allow_spinning: true` or `script_callers_per_session: 1` to get ORT's spinning
back (about 1 ms better serial latency for a single caller).

The legacy recommended layout for `script_workers: 2` on
10 cores, is **2 sessions × 4 threads, 4 callers, spinning off**. Measured on
the same host (10 cores, GLiNER2 int8, 36 texts) with 8 intra-op threads total:

| layout | c=1 p50 | c=4 p50 | req/s | sessions |
|---|---|---|---|---|
| 4×2 (default before this change) | 23.0 ms | 29.2 ms | 133 | 4 |
| 4×2, spinning off | 24.4 ms | 27.9 ms | 145 | 4 |
| **2×4, 4 callers, spinning off** | 19.4 ms | 26.7 ms | 164 | 2 |
| 1×8, 4 callers, spinning off | 18.7 ms | 29.5 ms | 146 | 1 |
| 2×4, 4 callers, spinning on | 15.8 ms | 30.0 ms | 118 | 2 |

`script_callers_per_session` does **not** add sessions — the thread budget above
counts distinct sessions, so sharing sessions is the memory-for-parallelism
saving, not a way to exceed the core budget.

### Runtime autotuning

With `capacity: auto` (the default) the server classifies each scripted model's
traffic every second as `idle`, `latency`, `throughput`, `saturated` or `unmeasured` from the
dispatch-wait/run-time ratio, in-flight count, and process CPU, and adapts the
per-session concurrency allowance within `script_callers_per_session`:

- `throughput` expands the allowance straight to the cap after one window, so
  a burst never queues behind a low allowance.
- `latency` halves it after ten consecutive windows (~10 s), so a short serial
  lull does not starve the next burst.
- `idle` holds the current allowance (no traffic is not evidence of a
  latency-sensitive workload).
- `saturated` never grows; it logs a recommendation to raise `script_workers`
  or `intra_op_threads` instead, because more concurrency cannot create CPU.
- `unmeasured` (runs completed but process CPU accounting was unavailable or no
  time elapsed) holds the allowance: without a CPU reading there is no headroom
  to grow into. CPU is normalized by the fractional effective bandwidth and the
  actual time between samples, so a saturated half-CPU quota reads as
  saturated.

The controller never changes the session count or per-session thread count at
runtime. `capacity: latency` or `throughput` pins the creation-time layout
without runtime adaptation; `autotune: off` keeps concurrency fixed. See
[operations](./operations.md#autotune-state) for the observable state.

## Image models and preprocessing parity

A model accepts `EMB.IMG` only when it declares an `image:` block. The block's
fields (`input`, `output`, `size`, `crop`, `resample`, `rescale`, `mean`, `std`)
are all optional: explicit configuration always wins, then
`preprocessor_config.json`, then the ONNX graph (a rank-4 image input's name and
static spatial dimensions), then documented defaults. An undeterminable `size`
is a load error naming the field. `crop: center` resizes the shortest edge then
center-crops (CLIP); `crop: none` resizes directly to `size×size` (SigLIP).

**Dual-encoder pairing.** When one model serves both `EMB` (text) and `EMB.IMG`
(image), the two branches must land in the same embedding space. The server
fails model loading when `dim` and the image output tensor's dimension disagree,
so a text-to-image search can never silently degrade to incomparable vectors.
Use a fused/paired export (e.g. `onnx-community/siglip2-base-patch16-224-ONNX`)
rather than mixing an unrelated vision checkpoint with a text model.

**Preprocessing parity caveat.** The Go preprocessing pipeline is within a
documented tolerance of the Python/Pillow reference, not bit-identical (their
resize kernels differ). Embeddings computed by `emb` are therefore not expected
to match a Python pipeline element-for-element. **Do not mix pipelines for one
index:** if documents were indexed with Python preprocessing, do not query with
`emb` (or vice versa) — pick one and use it for both sides. Clients that need
exact reference parity should preprocess with their own pipeline and send the
tensor through `EMB.EVAL` + `emb.run`.

## Preloading scripts

Scripts normally enter the cache via `EMB.SCRIPT LOAD`, which costs a client
round-trip and a server-side compile on first use. For deployments with known
scripts, declare them in model config and the server reads, validates, and
precompiles them at boot so `EMB.EVSHA` works immediately:

```yaml
models:
  minilm:
    model_repo: Xenova/all-MiniLM-L6-v2
    scripts:
      - scripts/classify.lua      # relative → config file's directory
      - /etc/emb/normalize.lua    # absolute, used as-is
```

A bad script (missing file, invalid Lua, oversized) is **fatal at boot** — the
server refuses to start half-configured, matching existing model validation
semantics. Preloaded scripts are indistinguishable from dynamically loaded
ones: `EMB.SCRIPT EXISTS` reports `1`, `EMB.EVSHA` replies immediately, and
`EMB.SCRIPT FLUSH` drops them like any other cache entry.

## Batching

Batching is **on by default** for every model, so no config is needed for the
performance path: a window coalesces concurrent requests (including `EMB.MULTI`
pairs) into shared ONNX runs, a token budget bounds each run, and dedicated
tokenizer workers hide tokenization behind inference. `timeout: 0` opts out.
`EMB.MULTI` processes pairs with bounded concurrency (≤ the machine's `GOMAXPROCS`),
so request storms can't spawn unbounded goroutines that starve inference.

## Batch determinism (automatic, no config)

Embeddings served by a batched model are guaranteed to be a deterministic
function of the input text: the same text returns **byte-identical bytes**
regardless of which other texts share its inference batch. Most graphs satisfy
this out of the box — fp32 exports, and int8 exports with static (baked)
activation scales (QDQ). Dynamic-quantized int8 exports do **not**: they
derive activation scales from the whole batch tensor's min/max
(`DynamicQuantizeLinear`), so each text's vector shifts slightly depending on
batch composition.

No configuration is needed — determinism is enforced by construction at load
time. Whenever a model loads with a batching window enabled, emb runs a
one-shot probe (two canned texts, embedded once each alone and once co-batched,
byte-compared; milliseconds, cached for the model's lifetime):

- **Probe passes** (fp32 / static-QDQ graphs): batching stays enabled.
- **Probe fails** (dynamic-quantized int8 graphs): the model automatically
  degrades to the unbatched worker pool (single-row inference — deterministic),
  with a logged warning; `batching.timeout: 0` is *not* required and provides
  no additional determinism.

There is no flag to re-enable batching on a failing graph. Operators who must
keep batching *and* determinism should serve a deterministic export (see
below), and can gate deployments on the boot-log degradation line:

```
batch_determinism=failed reason=dql_batch_dependence
```

The verdict is observable per model: `EMB.INFO <model>` reports
`batch_determinism` (`passed` | `failed` | `untested`) and
`batch_determinism_reason`, and the effective `batching_timeout_ms` (0 when a
model degraded or was configured unbatched). `EMB.STATS` reports the verdict
per loaded model. `untested` means batching is off by config (`timeout: 0`), so
nothing needed gating.

### Keeping int8 size *and* batching (deployment guidance)

If the int8 dynamic-quantized export fails the probe, the documented path to
regain batching without quadrupling memory is a **static-quantized (QDQ)**
export: activation scales baked as constants from a calibration pass (optimum `ORTQuantizer.fit` with an `is_static=True` quantization config over a representative corpus, then `ORTQuantizer.quantize`), which
contains no batch-sensitive op and passes the probe. fp32 exports are the
zero-effort alternative (larger, slower, but batch-invariant and
byte-deterministic).

## Caching

Embeddings are cached by `model:text` key in an in-process LRU, so repeated
texts skip ONNX inference entirely. Configure via the `cache:` YAML key or the
`-cache` CLI flag:

| Value | Behavior |
|-------|----------|
| *(empty)* | Cache disabled (default) |
| `auto` | ~13% of total RAM: 20% of memory left after a 10% safety margin and a 25% model reserve, floored at 64MB. No fixed byte cap — scales with the machine |
| `1GB`, `256MB`, … | Explicit byte budget (`docker/go-units` sizes) |
| `25%` | Percentage of total system RAM (explicit operator choice — no safety margins applied) |

```bash
./bin/emb -config config.yaml -cache auto   # ~13% of RAM on this machine
./bin/emb -config config.yaml -cache 25%    # a quarter of RAM
./bin/emb -config config.yaml -cache 2GB    # fixed budget
```

Invalid sizes or percentages (e.g. `150%`, `abc`) fail startup with a clear
error. Live cache stats — hits, misses, hit rate, evictions, entries, and byte
usage — are visible per model via `EMB.INFO <model>` and globally via `INFO` and
`EMB.STATS`. See [BENCHMARK.md](../BENCHMARK.md) → *Cache* for hit-rate
measurements.

Entries include key, value, and per-entry overhead in the byte budget. A write
that cannot fit by itself is skipped without evicting entries or replacing an
existing value; inference still returns its computed result. Admissible growing
replacements evict least-recently-used entries as needed. This bounds accounted
live cache storage, not process RSS or values retained by in-progress snapshots.

## Persistent cache snapshots

Snapshots optionally preserve the in-process LRU across restarts. They are
inspired by Redis RDB operationally, but are a small emb-specific streaming
format and are **not RDB-compatible**. Persistence is completely dormant when
`cache_file` is empty: no coordinator, timer, fingerprinting, serialization,
filesystem work, or cache hot-path checks are created.

| Setting / CLI flag | Default | Live update | Meaning |
|---|---:|---:|---|
| `cache_file` / `-cache-file` | empty | yes | Snapshot path; empty disables persistence |
| `cache_load` / `-cache-load` | `true` | restart | Restore at startup when the file exists |
| `cache_save` / `-cache-save` | empty | yes | Positive automatic-save interval; empty disables periodic saves |
| `cache_save_on_shutdown` / `-cache-save-on-shutdown` | `true` | yes | Save a dirty cache after inference drains and before model close |
| `cache_restore_limit` / `-cache-restore-limit` | `auto` | restart | Maximum restore memory as `auto`, bytes, or percentage of host RAM |
| `cache_restore_reserve` / `-cache-restore-reserve` | `10%` | restart | Host memory kept free during restore |
| `cache_save_rate_limit` / `-cache-save-rate-limit` | `0` | yes | Output rate such as `100MB/s`; `0` is unlimited |

Startup restore streams into an unpublished staging cache. Its effective
ceiling is the smallest of the configured cache budget, `cache_restore_limit`,
and sampled host headroom (`total RAM - current RSS - reserve`). Compatible
entries are admitted MRU-first; checksum failure leaves the live cache empty.
Model/tokenizer fingerprints are streamed once and cached, so periodic saves
do not repeatedly read model artifacts.

The script reply-key identity upgrade intentionally makes legacy script replies
cold: requests recompute them, while compatible text/image entries still hit.
The snapshot format is unchanged; restored legacy script entries remain bounded
and leave through normal eviction. Current script replies survive save/restore.
No legacy-key fallback is used, and rolling back restores the old cache bugs.

Automatic and manual saves briefly capture immutable entry descriptors under
the cache mutex, then encode, checksum, throttle, sync, and rename in a
background goroutine. Inference and all database commands continue during
that work. Only one save runs at a time; clean timer ticks are coalesced and
`EMB.SAVE` reports an overlap error. Files are written through an owner-only
`0600` temporary file and atomically renamed, preserving the prior snapshot
until the replacement is complete.

Snapshot files contain original input text and embedding bytes. Protect or
encrypt their storage as the source data requires. They are a disposable
warm-start optimization, not a durable database or backup; deleting a bad or
incompatible file is always safe.

Live changes to `cache_file` and the `EMB.SAVE` command require either a
configured `password` or a loopback-only `listen` address. The default
listener (`:6379`) binds every interface, so with no `password` set the server
rejects those commands with an explicit error rather than letting an
unauthenticated client point snapshot writes at an arbitrary path. Bind
`localhost` for a password-free deployment, or set `password` (and `AUTH`)
when serving non-loopback clients.

## Telemetry (OpenTelemetry)

`emb` can export its metrics as OpenTelemetry signals over OTLP/HTTP, so a
collector or a vendor backend (Datadog, for example) can pick them up without a
bespoke RESP poller. Export is **off** unless an endpoint is configured; with no
endpoint the server makes no outbound connection.

Exported counters and gauges come from the same aggregation that feeds `INFO`
and `EMB.STATS`, so they cannot drift from the commands. Counters are exported
with **delta** temporality (the increase since the previous export); gauges
report their current value. Process metrics use OpenTelemetry semantic-convention
names (`process.memory.usage`, `process.cpu.time`, `process.uptime`); server
metrics use an `emb.` prefix (`emb.requests`, `emb.tokens`, `emb.errors`,
`emb.active_requests`, `emb.connections`, `emb.cache.*`, `emb.models.loaded`,
`emb.truncated.*`, `emb.net.*`). Per-model series carry a `model` attribute.

Per-model inference timing is exported as cumulative sums, never pre-averaged:
`emb.dispatch.wait` and `emb.inference.duration` (seconds spent waiting for an
inference slot and doing inference work), `emb.inference.runs`, and the separate
scripted-evaluation series `emb.script.dispatch.wait`,
`emb.script.inference.duration` and `emb.script.inference.runs`. Consumers
derive mean queue and inference times by dividing those sums by
`emb.inference.runs` (or the matching `emb.script.inference.runs`).
`emb.sessions.busy` and `emb.sessions.total` report the model's current session
occupancy, and `emb.padding.efficiency` its embedding tokens consumed divided by
token slots processed. A model with no embedding pool exports none of the
embedding timing or session series.

`emb.request.duration` is an explicit-bucket **histogram** of completed-request
latency in seconds, carrying the `model` attribute and delta temporality: each
datapoint's count, sum and per-bucket counts describe one export interval alone,
and the first export after startup emits none. The boundaries step roughly
2-2.5x from 50µs to 10s and the final bucket is the `+Inf` overflow, so p50/p95/
p99 are available to any backend without reading the bounded `MONITOR` ring. It
is measured directly as each request completes, so its population spans every
inference command (`EMB`, `EMB.MULTI`, `EMB.IMG`, `EMB.IMGMULTI`, `EMB.EVAL`,
`EMB.EVSHA`) and includes failed requests. It therefore does not cover the same
population as `emb.requests`, which counts the text pool only — do not expect
the histogram's count to equal a model's `emb.requests` delta.

| Setting / CLI flag | Default | Live update | Meaning |
|---|---:|---:|---|
| `telemetry.endpoint` / `-otel-endpoint` | empty | restart | OTLP/HTTP endpoint; empty disables export |
| `telemetry.interval` / `-otel-interval` | `30s` | restart | Export interval |
| `telemetry.headers` / `-otel-header` (repeatable) | none | restart | Extra request headers, `key=value` |
| `telemetry.service_name` / `-otel-service-name` | `emb` | restart | `service.name` resource attribute |
| `telemetry.host` / `-otel-host` | hostname | restart | `host.name` resource attribute |
| `telemetry.attributes` | none | restart | Extra resource attributes |
| `telemetry.enabled` / `-otel-disabled` | `true` | restart | Explicit on/off |

The standard OpenTelemetry environment variables are also honored:
`OTEL_EXPORTER_OTLP_ENDPOINT`, `OTEL_EXPORTER_OTLP_METRICS_ENDPOINT`,
`OTEL_SERVICE_NAME`, `OTEL_RESOURCE_ATTRIBUTES`, `OTEL_METRIC_EXPORT_INTERVAL`
(milliseconds), `OTEL_EXPORTER_OTLP_METRICS_HEADERS`, and `OTEL_SDK_DISABLED`.
When a setting comes from more than one source, a CLI flag wins over the YAML
`telemetry:` block, which wins over the environment. `OTEL_SDK_DISABLED=true`
disables export regardless of the other sources.

```bash
# OTLP/HTTP to a local OpenTelemetry Collector (or the Datadog Agent on :4318)
./bin/emb -config config.yaml -otel-endpoint http://localhost:4318

# same thing through the environment
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318 ./bin/emb -config config.yaml
```

A bare endpoint gets the standard `/v1/metrics` path appended. An endpoint that
already carries a path (for example Datadog's direct intake) is used as-is.
`CONFIG GET otel*` reports the effective settings; they are read-only.
