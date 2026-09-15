## Context

See `proposal.md` — Why. The constraints that shape the design:

- The clients are browser pages with no raw TCP; the server speaks RESP over
  TCP. Something must bridge them.
- `emb` has no hosted surface and `PRODUCT.md` says none exists, so this is a
  sandbox that resets, not a service.
- `internal/resp` is an existing CGo-free RESP2 client shared by `emb-top` and
  the verifiers; `emb`'s RESP3 is per-connection and complete (spec
  `resp3-protocol`).
- Preset scripts are already a server feature: `models.<name>.scripts` preloads
  Lua and `PreloadScript` returns the digest.
- Model loading happens before the listener binds; `SetReady` is called before
  `Start`, so "can connect" is "ready".
- The site is served from a static Worker at `emb.is`; `website/` is the publish
  directory, filtered by `.assetsignore` and asserted by `published-tree.py`.

## Goals / Non-Goals

**Goals:**

- Real replies from a real `emb` process reach a browser, with the server
  unreachable from the internet.
- One HTTP contract that both the landing console and a standalone terminal
  speak, so replies cannot render two ways.
- A cost ceiling that holds no matter how many strangers arrive.

**Non-Goals:**

- A general Redis-protocol gateway. The surface is fixed and read-only.
- Image embeddings, monitoring, script authoring, or configuration changes.
- Persistence of cache or scripts across restarts; the sandbox may reset.
- Changing the server binary or any server-side capability spec.

## Decisions

**D1 · One public bridge; `emb` on loopback.** The bridge is the only listener
reachable from the platform's proxy; `emb` binds `127.0.0.1`. Because the server
sees a loopback peer, no password is needed and none reaches the browser.
_Alternatives:_ public `emb` + a browser-held `AUTH` secret (secret ships to the
client; hostile RESP reaches the server); a Cloudflare edge in front (still
needs an origin bridge, so it only adds a deploy and a failure mode).

**D2 · HTTP per command; no client sessions.** Each request carries
`{args, proto}` and returns one reply. There is no per-visitor state to keep, so
nothing is lost between requests. _Alternative:_ a WebSocket session — holds the
machine awake, needs a reconnect story, and buys nothing once presets replace
user scripts.

**D3 · One serialized upstream connection that toggles `HELLO 2|3`.** RESP is not
multiplexed, so one connection carries one in-flight command; the bridge holds a
mutex. The protocol version is a property of that connection, and the bridge
re-sends `HELLO` per request as needed, so the RESP2 and RESP3 forms are
genuinely the server's. _Alternatives:_ a connection pool (more state, no
throughput gain on one vCPU); always-RESP3 with the RESP2 form synthesized
(client-side fakery, refused by the `product-site` requirement).

**D4 · Extend `internal/resp` with the RESP3 kinds `emb` emits.** Production
handlers emit only `%` (map), `,` (double) and `_` (null) beyond the RESP2
kinds; `~ # ( = >` are wired for byte accounting but never sent. Adding those
three prefixes keeps the existing client as the single RESP implementation and
lets `emb-top` read RESP3 too. _Alternatives:_ a decoder inside the bridge
(duplicates the codec); RESP2-only with a VALUES-only render (the console could
not show the typed form the spec requires).

**D5 · Fixed allowlist plus preset digests.** The bridge validates the whole
argv shape and refuses `CONFIG`, `AUTH`, `MONITOR`, `EMB.SAVE`,
`EMB.CACHE.FLUSH`, `EMB.SCRIPT *`, `EMB.IMG*`, `SHUTDOWN`. `EMB.EVSHA` is
accepted only for digests in the preset manifest. _Alternative:_ forward
everything and rely on the server's own limits — a visitor could then flush
shared state or read other visitors' traffic via `MONITOR`.

**D6 · Entrypoint script owns the unit.** One `run.sh` starts `emb` in the
background, starts the bridge in the foreground, forwards the stop signal to
`emb`, waits, and exits; the bridge refuses to answer commands until it can
connect to `emb`, and exits if `emb` is gone so the platform replaces the
machine. _Alternatives:_ the bridge supervises `emb` (works, but puts process
management in the HTTP service); `s6`/`supervisord` (a dependency for one child).

**D7 · Models on a volume, configured by absolute path.** The config names
`/data/models/<name>/model.onnx` so `downloadModel` writes onto the mounted
volume; the first boot downloads once, every later boot loads from disk.
_Alternative:_ bake models into the image (smaller cold start, but couples the
image to the model set and the repo already ships a volume-backed pattern).

**D8 · The machine stays running.** `min_machines_running = 1`,
`auto_stop_machines = "off"`. Cost was explicitly accepted in exchange for
removing the cold-start path, the wake-and-retry client state, and the
proxy-timeout cliff on first download. The budget breaker, not autostop, is the
cost ceiling.

**D9 · Reply envelope.** One JSON object per reply, discriminated by kind, so a
client never re-parses a wire format:

```
{ "kind": "bulk"|"status"|"int"|"double"|"nil"|"error"|"array"|"map",
  "text": "...",            // bulk/status/error, when valid UTF-8
  "b64": "...",             // bulk whose bytes are not text
  "float": 0.5,             // double
  "int": 2,
  "elems": [ ... ],         // array/map values, recursively
  "vector": { "dtype": "float32", "count": 384 } }   // when a bulk is an embedding
```

A BLOB embedding is `kind: "bulk"` with `b64` and a `vector` descriptor; the
browser renders a preview from metadata without decoding the model. `VALUES`
arrives as `kind: "map"` under RESP3 and a flat `array` under RESP2, so the
protocol difference is visible by construction. _Alternative:_ base64 the whole
reply and let JS decode — reintroduces a RESP parser in the browser, which the
old transcript client was built to avoid.

**D10 · One client module, served by the service.** The terminal client is the
canonical implementation; the landing page loads the same module. Because the
site has no build step and the service cannot `go:embed` a path above its own
directory, the module lives under the service directory and is referenced from
the landing page at the service origin, with a check asserting the referenced
file is served. _Alternative:_ two copies kept in sync by a byte-equality check
(more moving parts); serving the terminal only from `emb.is` (drops the
standalone surface the proposal keeps).

**D11 · In-memory budget breaker.** Per-IP buckets, a global concurrency
semaphore, and a rolling inference-work ceiling live in the bridge's memory. One
machine means global state is genuinely global; a restart resets the ceiling,
which is acceptable for a sandbox and avoids a storage dependency.

**D12 · Endpoints.** `POST /api/exec` (the one command path), `GET /api/health`
(process up), `GET /api/ready` (server answers), `GET /` (standalone terminal),
`GET /terminal.js`. The console's offline/starting states map to transport
failure and to `ready: false` respectively.

## Risks / Trade-offs

- **Hostile traffic on a public endpoint** → whole-argv validation, per-IP and
  global limits, size and text caps, and a total-work breaker; the server is
  never reachable directly.
- **The classifier used as an `EMB` target may not auto-configure** (its output
  is `logits [-1,2]`, so it needs `pooling: none`, `normalize: false`, `dim: 2`)
  → spike this first; if it fails, `EMB.MULTI` falls back to two
  sentence-transformer models and the script preset moves to the second model's
  own task.
- **Preset digest drift between the site and the loaded script** → one canonical
  preset directory is the source for both the server config and the displayed
  digest, and a check compares them, mirroring `stamp-version.py`.
- **`website/repl` shipping as static assets** → `.assetsignore` entry plus the
  `published-tree.py` expected set, asserted before deploy.
- **A site-only CI classification hides broken service Go** → the service
  directory is classified as code and runs its tests.
- **Cross-origin requests from `emb.is` to `cli.emb.is`** → the bridge answers a
  fixed allowlist of origins; CORS is not treated as a security boundary.
- **Memory pressure from two models on a small machine** → measure RSS at
  startup; size the machine to the measured pair, not to a guess.
- **A long inference on a single shared vCPU** → per-request deadline and a
  bounded concurrency, with the timeout mapped to a legible reply.

## Migration Plan

1. Ship `internal/resp` RESP3 decoding and the bridge with the command surface
   disabled at the edge; verify against a local `emb`.
2. Enable the Fly app and volume, confirm the loopback-only bind and limits on
   the `cli.emb.is` host.
3. Enable the console and standalone terminal; keep the placeholder's markup
   available in history as the rollback. Rollback is a normal Fly release
   rollback plus re-withholding the console; the server and site are untouched.

## Open Questions

None that change the specs or the task set. The model pair, the exact request
and text caps, and the breaker's ceiling are tuning values to set from the
startup measurement.
