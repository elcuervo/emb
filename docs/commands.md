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
| `EMB.INFO <model>` | Model details: dim, workers, requests served, avg latency, live cache stats |
| `EMB.STATS` | Server statistics: uptime, total requests, live connections, active requests, per-model breakdown, mem (RSS MB), cpu user/sys usec, goroutines |
| `MONITOR [seq] [limit]` | Recent completed-request events (`seq`, timestamp µs, model, texts, latency µs, error) from a bounded ring. Incremental (`seq`) fetch; no text payloads |
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
| `AUTH <password>` | Authenticate the connection (required if `password` is set) |
| `HELLO [2\|3]` | Negotiate the RESP protocol version for the connection (default 2); bare `HELLO` reports the current version |
| `PING` | PONG |

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
to null slots without being decoded. See [Configuration](configuration.md).

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
