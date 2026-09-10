# How emb works internally

This is the server-side tour: what happens between a command arriving on the
RESP port and its reply going back out. Every diagram below is a hand-authored
animated SVG sharing one style — see [AGENTS.md](../AGENTS.md) for the contract.

## The whole machine

<img src="assets/diagrams/emb-system-overview.svg" alt="Clients — the ruby emb gem, redis-cli or any RESP client, and go/rust/python clients — reach one emb process over a single RESP port. Inside the process the RESP surface handles protocol negotiation, auth, idle timeout and the connection cap, then command dispatch. Dispatch probes the LRU embedding cache by model:text; a hit returns the stored bytes to the reply writer without touching the model. A miss goes to the registry, which lazily loads the model into a pool of ONNX Runtime sessions, then to smart batching, which coalesces concurrent requests into one ONNX run. Results are written back into the cache and then to the reply writer, which emits either BLOB float32 bytes or a VALUES envelope depending on the format keyword and negotiated protocol. All three subsystems sit on CGo-linked native libraries — libonnxruntime and libtokenizers — plus model artifacts on disk or downloaded from the HuggingFace hub.">

The plain-text shape, for anything that can't render the animation above:

```
clients ──RESP──▶ RESP surface ──▶ dispatch
                                      │
                     ┌────────────────┼────────────────┐
                     ▼                ▼                ▼
                 LRU cache       registry/pool     smart batching
                     │                │                │
                     └── hit: reply    └── lazy load    └── one ONNX run
                         miss ─────────────▶──────────────▶
                                      │
                                      ▼
                    CGo: libonnxruntime · libtokenizers · weights
```

Three subsystems, and they are genuinely independent: the cache is optional and
sits in front of everything; the registry decides *whether* a model is loaded;
the batcher decides *how* the work is shaped once it is. Dispatch decides the
reply shape last, per call, from the format keyword and the negotiated protocol.

## One EMB command, end to end

<img src="assets/diagrams/emb-request-lifecycle.svg" alt="The life of a single EMB command. The socket decodes RESP2 or RESP3, the AUTH gate and the draining check run, then arity is validated and a leading reply-format keyword is read as a complete word at a fixed argument position — never inside the free-text tail. Texts beyond max_texts are dropped and their reply slots become nulls. The cache is probed with the key model:text; a hit jumps straight to the reply writer and skips both model loading and inference. A miss goes to the registry, where GetOrInit loads the model pool on first use — tokenizer, output tensor detection, sessions — and then to Pool.Embed, which hands the texts to the batching window that coalesces concurrent requests into one ONNX run. The run pads to the longest sequence in the window, pools (mean, cls, or none for pre-pooled graphs) and optionally L2-normalizes. Every miss is written back into the LRU cache and a MONITOR event is appended with the model, text count and latency. Finally writeEmbResult emits either the compact BLOB float32 reply or the VALUES envelope, sized per negotiated protocol; RESP2 and RESP3 differ in null and double encoding, and the overflow slots stay null in both.">

The two details that surprise people:

1. **The format keyword is read at a fixed position, never by scanning.** For
   `EMB` it is `args[2]`, for `EMB.MULTI` it is `args[1]`, and only when at least
   one payload argument follows. The free-text tail is never inspected, so a
   text ending in the word `VALUES` embeds normally.
2. **Truncation is a reply shape, not an error.** Texts past `max_texts` are
   simply not processed and their slots come back null, so one oversized
   command cannot pin the machine's cores.

## Smart batching

<img src="assets/diagrams/emb-smart-batching.svg" alt="Requests arriving on many connections — plain EMB calls, EMB.MULTI pairs, scripted evaluations — are pushed onto a bounded request channel. Dedicated tokenizer producers encode them off the run path, so tokenization of later requests overlaps the inference of earlier batches, and hand the encodings to a second bounded channel. The run loop accumulates ready items into a window, adding each item's real token count to a budget. The window flushes on whichever of three triggers comes first: the request count reaching max_batch (32 by default), the accumulated token budget reaching max_batch_tokens (16384 by default), or the batching timer firing (1 millisecond by default). When the window is empty the loop takes an idle-flush fast path and runs the queued burst immediately, so a lone request pays no artificial delay. Every flush pads all encodings to the longest sequence in the window and performs exactly one ONNX Runtime session run, then splits the result back to each caller by remembered offsets. A single request is never split across runs; the window bounds work, not request atomicity. Because padding is per-window, a short text waiting behind a long one is charged for the long one's padding, and that ratio is reported as padding efficiency in EMB.STATS.">

Batching is on by default for every model, and the reason it is on by default
rather than opt-in is the idle-flush path: when the run loop has no window open,
the first pending request is served immediately along with whatever is already
queued, so a lone request never pays the 1 ms window as an artificial delay.
The window only forms when requests actually overlap.

The cost of the window is padding. A window pads every text to its longest
member, which is why the token budget exists: it bounds how much padding one
long text can impose on the short ones behind it. `padding_efficiency` in
`EMB.STATS` is that ratio.

## Model lifecycle

<img src="assets/diagrams/emb-model-lifecycle.svg" alt="A model starts as configuration — either a path to an ONNX file or a HuggingFace repo to download — and is registered at startup as a placeholder entry with no pool and nothing read from disk, unless preload is set, which loads it at boot. The first EMB command naming that model calls GetOrInit, whose sync.Once runs ensurePool exactly once; every other concurrent caller waits on the same Once rather than starting a second load. ensurePool builds a tokenizer from tokenizer.json, asks ONNX Runtime for the output tensors so it can pick one and read its rank, reads the input names and the weight bytes, and creates a session from those bytes per worker. It then builds a pool: smart batching by default with a 1 millisecond window, max_batch 32, a 16384 token budget and asynchronous tokenizer workers, or a round-robin worker pool when batching is disabled with timeout 0. Worker count is auto-tuned from available memory against the model's own size. A missing output tensor is auto-selected by rank and logged, the rank decides the default pooling — mean for a 3D last_hidden_state, none for an already pooled 2D output — and a quantized int8 weight file is preferred when present. A load failure is sticky: the Once never retries, so a broken model cannot cause a retry storm, and the error is returned to every caller. Cache snapshot entries restored from disk are quarantined and admitted only after the model loads and its fingerprint — a hash of the weights, the tokenizer and every option that changes emitted bytes — still matches.">

Models are registered at startup and loaded on first use, and the load is
guarded by a `sync.Once`, so a failing model fails once and returns the same
error forever rather than retrying on every request. Worker count is derived
from available memory against the model's own size: roughly half of RAM divided
by the weights plus 20% overhead, capped at the core count.

## Embedding cache and snapshots

<img src="assets/diagrams/emb-cache-persistence.svg" alt="On the hot path, every EMB and EMB.MULTI text is looked up in an in-process LRU under the key model:text. A hit returns the stored float32 bytes and skips inference entirely; a miss runs the batch and then writes each result back. The cache is a map plus an intrusive list: Get promotes an entry to most-recently-used, and Set evicts the tail while the byte budget is exceeded. The budget is either an explicit size such as 1GB, a percentage of system RAM such as 25%, or auto, which is roughly 13 percent of RAM. Per-model hit, miss, eviction, entry and byte counts are reported through EMB.INFO, EMB.STATS and INFO. Snapshots are completely dormant unless cache_file is set. Three triggers start a save: the cache_save interval, a manual EMB.SAVE, or shutdown when the cache is dirty. The save takes immutable entry descriptors under the cache mutex and then, in one background goroutine, encodes, checksums, throttles, fsyncs and atomically renames a 0600 temporary file, so the previous snapshot survives until the replacement is complete and inference never stops. Only one save runs at a time; clean timer ticks are coalesced and a manual save that overlaps reports an error. At startup the file is streamed into an unpublished staging cache whose ceiling is the smallest of the cache budget, cache_restore_limit and the sampled host headroom of total RAM minus current RSS minus the reserve. Entries are admitted most-recently-used first; a checksum failure leaves the live cache empty. Model and tokenizer fingerprints are hashed once and cached so periodic saves do not re-read model artifacts, and restored rows are quarantined until the model loads and its fingerprint still matches. Snapshot files hold the original input text and can be disabled at any time.">

Persistence is completely dormant unless `cache_file` is set — no coordinator,
timer, fingerprinting or filesystem work is created at all. When it is set, the
two things worth remembering are that a save never blocks inference (it captures
immutable descriptors under the mutex and does everything else in a background
goroutine), and that a restore is a *hint*, not a source of truth: the model and
tokenizer fingerprints must still match, or the restored rows are discarded.

## Where the code lives

| Concern | Package |
|---|---|
| Connection handling, command dispatch, reply shapes | `internal/server` |
| Batching, padding, pooling, ONNX session pool | `internal/pipeline` |
| Model registry, lazy load, auto-tuning, fingerprints | `internal/registry` |
| ONNX Runtime CGo bindings | `internal/onnx` |
| Tokenizer loading and encoding | `internal/tokenizer` |
| Scripted evaluation (`EMB.EVAL`) | `internal/script` |
| YAML config and validation | `internal/config` |
| HuggingFace downloads | `internal/hfhub` |
| Live dashboard client | `internal/embtop` |
