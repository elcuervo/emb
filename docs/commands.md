# Commands

Full reference for the `emb` command surface. See the
[README](../README.md) for install and quick start.

| Command | Description |
|---------|-------------|
| `EMB <model> [BLOB\|VALUES] <text> [text...]` | Embed one or more texts. Default `BLOB`: single text → bulk string, multiple → array of bulk strings (float32 bytes). `VALUES`: `dtype`/`shape`/`values` envelope with decimal values |
| `EMB.MULTI [BLOB\|VALUES] <model> <text> [<model> <text>...]` | Embed texts across different models in one call; per-pair `VALUES` envelopes (with `model`) or null on failure |
| `EMB.IMG <model> [BLOB\|VALUES] <bytes> [<bytes>...]` | Embed one or more images from raw JPEG/PNG/GIF/WebP bytes (each a binary-safe bulk). `BLOB`: single image → bulk, multiple → array with nulls for failed/truncated slots; `VALUES`: one `[m, dim]` envelope over processed images. URLs are rejected |
| `EMB.IMGMULTI [BLOB\|VALUES] <model> <bytes> [<model> <bytes>...]` | Embed images across different models in one call; MGET-style per-pair nulls, per-pair `VALUES` envelopes (with `model`) |
| `EMB.MODELS` | List loaded models with dimensions and status |
| `EMB.INFO <model>` | Model details: dim, workers, requests served, avg latency, batch determinism, CPU share/thread width/spinning, the embedding path's dispatch/admission/run timings and runs, and that model's own cache stats (hits, misses, rate, evictions, entries, bytes) |
| `EMB.STATS` | Server statistics: uptime, total requests, per-model breakdown, CPU allowance/budget/active peak, cache/connection limits, mem (RSS MB), CPU user/sys usec, goroutines |
| `MONITOR [seq] [limit]` | Recent completed-request events from a bounded ring: `seq`, `at_us` (µs), `model`, `texts`, `latency_us`, `err`, then the request's own attribution — `cached_texts` (how many of its texts the reply cache answered), `attributed` (whether the server measured its phases), `dispatch_us`, `admit_us`, `run_us`. Eleven fields in that closed order, a flat array under RESP2 and a map with the same names under RESP3; the five attribution fields are appended after `err`, so a client reading the original six positionally is unaffected. Incremental (`seq`) fetch; no text payloads |
| `EMB.READY` | Health check: `+OK` (ready), `-ERR <reason>` (loading, draining, no models) |
| `EMB.EVAL <model> <script> <numtexts> <text...> <arg...>` | Evaluate a Lua script once against a model (KEYS=texts, ARGV=args) |
| `EMB.EVSHA <model> <sha> <numtexts> <text...> <arg...>` | Evaluate a cached script by SHA (see `EMB.SCRIPT LOAD`) |
| `EMB.SCRIPT LOAD <model> <script>` | Compile, cache, and return the script's SHA1 |
| `EMB.SCRIPT EXISTS <model> <sha...>` | Which scripts are cached (1/0 per SHA) |
| `EMB.SCRIPT FLUSH [<model>]` | Clear cached scripts (all models when omitted) |
| `EMB.CACHE.FLUSH [model]` | Remove all cached embeddings, or only entries for one configured model; returns the removed count |
| `EMB.SAVE` | Accept an asynchronous cache snapshot; poll `EMB.STATS` or `INFO cache` for completion/failure |
| `EMB.HELP` | Command reference (includes the full script surface) |
| `INFO [section...]` | Redis-style INFO: `server`, `cache`, `keyspace`, `stats`, `memory`, `cpu`, `clients` |
| `CONFIG GET [glob]` / `CONFIG SET` | Read or live-tune runtime settings (see [Configuration](configuration.md)) |
| `SHUTDOWN [NOSAVE\|SAVE] [NOW] [FORCE] [ABORT]` | Stop the server with Redis's semantics (see [SHUTDOWN](#shutdown)) |
| `AUTH <password>` | Authenticate the connection (required if `password` is set) |
| `HELLO [2\|3]` | Negotiate the RESP protocol version for the connection (default 2); bare `HELLO` reports the current version |
| `PING` | PONG |

`EMB.INFO` and `EMB.STATS` report the process CPU plan chosen at startup.
`EMB.STATS` carries `cpu_allowance`, `cpu_allowance_source`, `cpu_capacity`,
`cpu_reserve`, `cpu_budget`, `cpu_active_tokens`, `cpu_peak_tokens`, and the
admission pressure `cpu_admission_wait_us`, `cpu_admission_waiters`,
`cpu_admission_timeouts`, `admission_refusals` and `admission_timeout_ms`. It
also reports the kernel's CPU we could not use: `cpu_throttle_periods`,
`cpu_throttled_periods`, `cpu_throttle_usec` and `cpu_steal_usec`.
`EMB.INFO` carries each model's `workers`, `intra_op_threads`,
`cpu_thread_share`, `cpu_share_reclaimed`, `allow_spinning`, and the
`embed_dispatch_wait_us` /
`embed_admission_wait_us` / `embed_run_us` split of the embedding path — the
calibrated layout when calibration selected it. Those three timings — and the
`dispatch_us`, `admit_us`, `run_us` a `MONITOR` event carries — are attributed
per run: a request accounts for every run it made (one per text on the
worker-pool path, one per batch window on the batched path) rather than only the
last, and the wait a run paid at the gate is credited to that run instead of
being read from the session that served it, so one caller's wait is never
charged to another's run and the cumulative `embed_run_us` and
`embed_admission_wait_us` never decrease. `cpu_share_reclaimed` is 1 when
the configured weights over-allocated the process budget and this model gave up
its reservation (it then runs on capacity its peers leave idle). `calibration_source` reports how
the layout was chosen (`calibrated`, `unresolved`, `fallback`, `policy`,
`legacy`, or `none` before it is decided) and `calibration_reason` carries the
reason an unresolved or fallback layout was served. See
[Startup calibration](configuration.md#startup-calibration) for defaults,
precedence, and fallbacks, and
[Operations](operations.md#reading-the-process-cpu-plan) for what each timing
does and does not include.

## SHUTDOWN

`SHUTDOWN [NOSAVE|SAVE] [NOW] [FORCE] [ABORT]` stops the server the way Redis
does. Flags are case-insensitive and may come in any order.

| Flag | Meaning |
|---|---|
| `SAVE` / `NOSAVE` | Write, or do not write, the final cache snapshot. They override `cache_save_on_shutdown`; `SAVE` with `NOSAVE` is a syntax error |
| `NOW` | Skip the grace period and stop clients immediately |
| `FORCE` | Accepted for Redis compatibility. emb's snapshot is a rebuildable cache, so a failed save never blocks exit and `FORCE` is always effectively set |
| `ABORT` | Cancel a pending shutdown; cannot be combined with any other flag |

Replies:

- `-ERR syntax error` for an unknown flag, `SAVE` with `NOSAVE`, or `ABORT`
  combined with any other flag. Nothing starts and the server keeps running.
- `+OK` to the aborting client when `SHUTDOWN ABORT` cancels a pending or
  running shutdown; `-ERR No shutdown in progress.` when there is nothing to
  abort.
- A shutdown caller receives **no reply**: it blocks (for the grace, then the
  drain) and its connection closes as the process exits, as with Redis. A
  second `SHUTDOWN` sent while one runs also blocks, and its flags are ignored.
- A blocked caller released by `ABORT` receives
  `-ERR Errors trying to SHUTDOWN. Check logs.`

The sequence is the same one SIGTERM and SIGINT run: the grace period
(`shutdown_timeout`, during which every command is still served and `EMB.READY`
replies `draining`), then the listening socket closes and clients stop —
idle connections close at once, a running command finishes and its connection
closes right after the reply, and commands not yet executing get no reply at all
(their connection closes, so clients see a connection error, never a RESP error).
Then in-flight inference drains, the final snapshot is attempted within the
drain budget, telemetry flushes, and ONNX Runtime resources are released.

Access control follows Redis protected mode: with a `password` configured,
`SHUTDOWN` requires authentication like any other command; without one, only
loopback clients may call it, and any other client gets
`-DENIED SHUTDOWN is only accepted from loopback clients when no password is
configured`.

## EMB.MULTI

`EMB.MULTI` embeds texts against different models in a single round trip and
answers with MGET-style partial failures: each reply is the vector for its
model, and a failed pair yields an error in its slot without failing the rest.

```
redis-cli EMB.MULTI minilm "hello" siglip2 "a photo of a cat"
1) \x7c\x8e\x80\xbd...   (minilm, 384 floats)
2) \x4a\x9f\x31\xc2...   (siglip2, 768 floats)
```

## Image embeddings: EMB.IMG

`EMB.IMG` embeds images directly from their **encoded file bytes** — no base64,
no URL, no client-side preprocessing. RESP bulk strings are binary-safe, so a
client just sends the file:

```bash
# redis-cli -x reads the last argument from stdin:
cat cat.jpg | redis-cli -x EMB.IMG siglip2

# multiple images → one embedding slot each (null on a failed/truncated slot):
redis-cli EMB.IMG siglip2 <cat.jpg bytes> <dog.png bytes>

# cross-model, MGET-style per-pair nulls:
redis-cli EMB.IMGMULTI clip <cat.jpg bytes> siglip2 <dog.png bytes>
```

The server decodes the image (PNG/JPEG/GIF/WebP), resizes/crops, rescales, and
normalizes it to the model's `pixel_values` tensor (`[1, 3, H, W]` float32 RGB),
then runs **one batched inference** for all images in the command. The reply is
the same `BLOB`/`VALUES` grammar as text (`EMB.IMG` takes the keyword at
position 2, `EMB.IMGMULTI` at position 1).

**Bytes only — the server never fetches URLs.** An `http(s):` or `data:` URI
argument is rejected with an error telling the client to fetch the image and
send its bytes. This keeps the request path network-free (no SSRF surface) and
makes image caching optimal: entries are keyed `img:<model>:sha256(bytes)`, so a
changed image is always a different key and a hit costs no decode or network.

Image limits are configurable and enforced before decode/inference:
`max_images` (default 4096, `0` = unlimited), `max_image_bytes` (default 32 MiB),
`max_image_pixels` (default 33.5 MP, checked from the header), and the
command-wide `max_command_bytes` (default 64 MiB). A single bad or oversized
image fails only its own slot; overflow images past `max_images` are truncated
to null slots without being decoded. The command's one batched run must return
one output row per image; an output whose batch dimension disagrees is refused
(the server never pools or reads past the tensor) with
`image output "<tensor>" has batch dimension <received>, want <expected>`.
See [Configuration](configuration.md).

## Reply formats: BLOB and VALUES

`EMB` and `EMB.MULTI` accept an optional leading reply-format keyword,
mirroring RedisAI's `AI.TENSORGET <key> [META] [BLOB|VALUES]` (`EMB.IMG` takes it
at position 2, right after the model; `EMB.IMGMULTI` at position 1):

- **`BLOB`** (default) — the compact binary wire: raw little-endian float32
  bytes as bulk string(s). Fastest, and byte-identical to prior emb versions.
- **`VALUES`** — a self-describing envelope: `dtype` (`FLOAT`), `shape`
  `[m, dim]`, and a flat row-major `values` array of the embeddings as decimal
  floats. Handy for clients, tooling, and debugging that cannot decode raw
  float32 bytes.

```bash
redis-cli EMB minilm VALUES "hello world"
1) "dtype"
2) "FLOAT"
3) "shape"
4) 1) (integer) 1
   2) (integer) 384
5) "values"
6) 1) "-0.19744610786437988"
   2) "0.17766517400741577"
   ...
```

`EMB.MULTI ... VALUES` returns one envelope per pair (including a `model`
key, since per-pair dimensions differ across models), with nulls for failed
pairs. The keyword is detected only at its fixed position right after the
model name (or before the pairs) — it is never scanned from the text tail — so
trailing text that happens to read `BLOB` or `VALUES` embeds normally. The one
collision is a **first** text in a multi-text call: `EMB m VALUES hello`
treats `VALUES` as the keyword, so to embed the literal word first write it
twice — `EMB m VALUES VALUES hello` (or reorder the texts). As a safety
measure, `BLOB` and `VALUES` are reserved and cannot be used as model names.

## RESP3 and protocol negotiation

Connections speak RESP2 by default. A client can upgrade a connection to
RESP3 with `HELLO 3` (and back with `HELLO 2`); bare `HELLO` reports the
current version. Under RESP3:

- `EMB ... VALUES` values are typed RESP3 doubles (`,<decimal>`) instead of
  decimal bulk strings.
- `EMB.INFO`, `EMB.STATS`, `EMB.MODELS`, and `CONFIG GET` reply with maps
  instead of flat key/value arrays; nulls encode as `_` instead of `$-1`.
- `INFO` stays a bulk string in both protocols (as in real Redis), and errors
  stay simple errors.

The Ruby client ([`gems/emb`](../gems/emb/README.md)) keeps RESP2 and the binary
`BLOB` default; pass `format: :values` to opt into decimal replies. See
[Clients](clients.md).
