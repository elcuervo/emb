## Why

The site's best argument is withheld. `product-site` requires the console panel
to stay out of the rendered page until a live executor exists, so the shipped
revision ships a finished panel behind `hidden` and a transcript client that
replays hand-written lines. A visitor who already operates Redis is asked to
believe the bytes are real before running anything.

This change builds the missing runtime: a small HTTP showcase service beside
`emb` on Fly, reachable at `cli.emb.is`, driving the landing console and a
standalone terminal with replies from an actual `emb` process. The server
implementation does not change — only how a stranger is allowed to reach it.

## What Changes

- **New `website/repl/` service.** A small Go HTTP bridge is the only public
  surface; `emb` listens on loopback behind it in the same Fly machine, started
  by one entrypoint script. `emb` is never reachable from the internet.
- **A fixed showcase surface, not a general server.** The bridge exposes a
  read-only command set — `PING`, `EMB`, `EMB.MULTI`, `EMB.MODELS`, `EMB.INFO`,
  `EMB.STATS`, `EMB.READY`, `EMB.HELP`, `INFO`, `HELLO 2|3`, and `EMB.EVSHA`
  for preloaded preset scripts only. `CONFIG`, `AUTH`, `MONITOR`, `EMB.SAVE`,
  `EMB.CACHE.FLUSH`, `EMB.SCRIPT *`, and `EMB.IMG*` are refused by the bridge.
  Nothing a visitor sends can change server configuration or shared state.
- **Two models on a Fly volume**, one embedding and one classifier, so `EMB`,
  `EMB.MULTI` (different dimensions in one round trip), and a scripted
  classification preset are all demonstrable.
- **RESP3 is real, not simulated.** The bridge holds an upstream RESP3
  connection (`HELLO 3`) and decodes typed replies; `internal/resp` gains the
  three RESP3 kinds `emb` actually emits (`%` map, `,` double, `_` null) so the
  service can read them. The console's `HELLO 3` / `VALUES` claim becomes true.
- **The landing console is enabled and wired to the service.** It stops
  replaying transcripts, gains a live state and an offline state, and keeps its
  markup, modes, keyboard behavior, and reduced-motion behavior.
- **A standalone terminal at `cli.emb.is`** driven by the same client module and
  the same API.
- **Abuse controls sized for one shared vCPU:** per-IP and global token buckets,
  a bounded concurrency semaphore, request and text size caps, and a global
  inference budget breaker that answers with a legible refusal instead of
  billing for a flood.
- **The publish boundary and CI absorb the new directory.** `website/repl/**`
  joins `.assetsignore` and the asserted served set; CI classifies it as code so
  the Go tests run even though it lives under the site directory.

## Capabilities

### New Capabilities

- `sandbox-service`: the public showcase runtime — the bridge and its fixed,
  read-only command surface, the request/reply contract the two clients speak,
  preset script ownership, the RESP3 upstream transport, the process model that
  keeps `emb` on loopback, and the limits that bound what a stranger can spend.

### Modified Capabilities

- `product-site`: the console requirement changes from "withhold a placeholder
  with a live seam" to "render a console driven by a live executor", and its
  states gain a real offline branch. The transcript and the "demo, not a live
  server" framing are removed once the executor is real.
- `site-deployment`: the served-tree deny list and the CI classification change,
  because the site directory now contains Go service code that must be tested
  and must not be published.

## Impact

- **New:** `website/repl/` (service, presets, Dockerfile or entrypoint, `fly.toml`,
  standalone terminal page), a new Fly app and `cli.emb.is` record.
- **Modified:** `website/index.html` (console section un-hidden and re-framed),
  `website/assets/js/main.js` (transcript engine removed, live executor and
  request tokenizer added), `website/assets/css/styles.css` (live/offline
  states), `website/.assetsignore`, `website/tools/published-tree.py`,
  `internal/resp/resp.go` (RESP3 decode), `.github/workflows/ci.yml`,
  `justfile` (sandbox run/build targets), `website/README.md` and `PRODUCT.md`
  (the "no hosted offering" line must still hold: this is a sandbox, not a
  service).
- **Operational:** one Fly machine, one volume, one bill; a second DNS name. No
  change to the server binary, its specs, or the gems.
