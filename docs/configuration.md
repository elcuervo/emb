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
| `onnx` | — | Path to ONNX model file |
| `tokenizer` | `<model-dir>/tokenizer.json` | Path to HuggingFace tokenizer JSON |
| `model_repo` | — | HuggingFace repo (auto-downloads ONNX + tokenizer) |
| `dim` | auto-detected | Embedding dimension |
| `max_length` | auto-detected (or 512) | Max token sequence length |
| `pooling` | auto-detected | `mean` (3D output), `cls` (first token) or `none` (2D pre-pooled) |
| `normalize` | `false` | L2-normalize the output |
| `output_tensor` | auto-detected | ONNX output tensor name |
| `preload` | `false` | Load model at startup instead of on first request |
| `pad_output` | `false` | Pad sequences to `max_length` with trailing zeros (compatibility with legacy implementations that don't pass attention mask) |
| `workers` | auto-tuned | Number of worker goroutines |
| `script_workers` | auto-tuned | Named-tensor sessions for scripted models (`EMB.EVAL`/`EMB.EVSHA`). `0`/absent auto-tunes from RAM and model size (min 1). With `intra_op_threads` unset, each session gets `max(1, (cores−2)/script_workers)` threads so `sessions × threads ≈ cores` |
| `script_callers_per_session` | `4` | Concurrent evaluations allowed per scripted session (the analyzed sweet spot). ORT sessions are safe for concurrent `Run`, so a lone scripted model is not serialized behind one session. `0`/absent = 4; set `1` to serialize |
| `allow_spinning` | follows sharing | Maps to ORT `session.intra_op.allow_spinning`. When unset, scripted sessions spin off while shared (`script_callers_per_session > 1`) so concurrent callers stop fighting for cores, and keep ORT's default otherwise. Embedding and image sessions keep ORT's default unless set explicitly |
| `autotune` | `auto` | `auto`/`callers` enable the runtime concurrency controller; `off` fixes concurrency at the configured value. Applies to scripted sessions |
| `capacity` | `auto` | Creation-time layout: `auto` (derived layout plus runtime adaptation), `latency` (spinning on, one caller per session), or `throughput` (spinning off, shared sessions) |
| `intra_op_threads` | `cores−2` (shared) | ONNX intra-op threads per session. Embedding and image sessions use `cores−2`; scripted sessions divide that budget across `script_workers` (`max(1, (cores−2)/script_workers)`). An explicit value is honoured verbatim for every path. At boot the server logs a warning when the total budget (`sessions × threads` across all models) exceeds the core count |
| `scripts` | `[]` | List of file paths to Lua scripts to preload at boot. Relative paths resolve against the config file's directory; absolute paths are used as-is. Invalid scripts (bad syntax, missing file, oversized) fail startup |
| `image` | — | Image preprocessing block (enables `EMB.IMG`); see [Image models](#image-models-and-preprocessing-parity) |
| `image_preload` | `false` | Warm the image named-session pool at startup instead of on first `EMB.IMG` |
| `batching` | `{timeout: 1, max_batch: 32, max_batch_tokens: 16384}` | Smart batching settings. **Enabled by default** (1 ms window) for every model; set `timeout: 0` to use the worker pool. With batching on, `tokenize_workers` defaults to `min(4, cores)` and the token budget auto-applies. Batching is batch-determinism gated automatically (no flag): dynamic-quantized int8 graphs degrade to the worker pool at load (see *Batch determinism*) |

## Thread budget

ONNX Runtime allocates `intra_op_threads` per session. Summed across every
session of every model, that budget should stay near the core count, or the
sessions contend for the CPU and throughput falls while tail latency rises.
The rule is:

```
workers × threads ≈ cores
```

When `intra_op_threads` is unset, scripted sessions divide the `cores−2` budget
across `script_workers`: `max(1, (cores−2)/script_workers)`. An explicit
`intra_op_threads` is honoured verbatim for every path, and the server warns at
boot when the total configured budget exceeds the core count, naming each
contributing model with its session and thread counts. The warning never
changes configuration.

The budget is sized from the process's **effective** limits, not the host's: in
a container (Fargate, EKS, Docker) `workers` uses the cgroup memory limit and
the derived thread count uses the cgroup CPU quota. A task that reports the
host's 32 GB therefore cannot open a session pool sized for 32 GB and get OOM
killed.

Measured on the reference host (10 cores, GLiNER2 int8, 36-text corpus, no
reply cache) at concurrency 8:

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

Out of the box, each scripted session is shared by `script_callers_per_session`
(4) evaluations, and spinning is off while a session is shared. A lone scripted
model therefore does not serialize behind one session: measured on the
reference host, one session × 8 threads with 4 callers and spinning off gives
146 req/s where the old `cores−2`-per-session default gave 24. Set
`allow_spinning: true` or `script_callers_per_session: 1` to get ORT's spinning
back (about 1 ms better serial latency for a single caller).

The recommended layout, which the defaults produce for `script_workers: 2` on
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
traffic every second as `idle`, `latency`, `throughput`, or `saturated` from the
dispatch-wait/run-time ratio, in-flight count, and process CPU, and adapts the
per-session concurrency allowance within `script_callers_per_session`:

- `throughput` doubles the allowance toward the cap after two consecutive
  windows (readiness over raw numbers: 1 → 2 → 4).
- `latency` halves it after three consecutive windows.
- `idle` holds the current allowance (no traffic is not evidence of a
  latency-sensitive workload).
- `saturated` never grows; it logs a recommendation to raise `script_workers`
  or `intra_op_threads` instead, because more concurrency cannot create CPU.

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
