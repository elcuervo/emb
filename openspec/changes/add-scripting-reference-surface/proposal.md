# Proposal

## Why

The scripting engine is `emb`'s second surface — a sandboxed Lua host with its
own commands, its own budgets, its own reply grammar and its own reference
implementations — and the site carries it in three partial places, none of them
in full. The landing's scripts block argues the shift in one specimen; the demos
plate "the model is a function" teaches the idea; the docs surface's
`05 · Lua scripting` lists roughly a third of the host surface. A reader who
wants the complete property set — every `emb.*` call, the tensor spec, the reply
grammar, the budgets, the cache identity, the sandbox — has no page to open.

Two versions make it worse. The server reports one version (`VERSION`, injected
by `main`), and the script host carried a second (`emb.API_VERSION = "1.3.0"`)
with a hand-maintained history of its own. The site would have to stamp both,
and a script checking capability would be comparing a number the server does not
otherwise use.

## What Changes

- **Add a reference surface at `website/scripting/`.** A Read surface, linked in
  the primary nav beside Docs, Demos, and Gem, carrying the complete scripting
  engine: commands, preloading, the sandbox, the full `emb.*` API, tensors,
  replies, budgets, and cache identity. Built from the existing atoms and the
  shared stylesheet; no new visual language.
- **Unify the version.** `emb.API_VERSION` becomes the server version:
  `VERSION` → `main.version` → `Server.SetVersion` → `script.APIVersion`, with
  the same value folded into reply-cache identity. The independent 1.x host-API
  history is dropped.
- **Reconcile `05 · Lua scripting`** with the new surface: it keeps the narrative
  and the worked example, and links to the reference for the complete form, so
  the scripting fact has one complete home instead of two partial ones.
- **Ship the support tooling**: the nav entry on every page,
  `published-tree.py`, `stamp-version.py`, and an Impeccable surface brief.

## Impact

- Affected specs: `scripting-reference` (new), `script-eval` (modified),
  `product-docs` (modified), `product-site` (modified).
- Affected code: `internal/script/version.go` (new shape), `internal/script/host.go`,
  `internal/script/cache.go`, `internal/server/server.go` (`SetVersion`),
  `internal/script/compat_test.go`, `internal/script/cache_test.go`.
- Affected site: `website/scripting/index.html` (new), the four nav bars,
  `website/docs/index.html` (`05 · Lua scripting` reconciliation),
  `website/tools/published-tree.py`, `website/tools/stamp-version.py`,
  `website/.impeccable/surfaces/`.

## Downstream behavior change

Folding the server version into the reply-cache key means **every upgrade
invalidates cached script replies**, including releases that do not touch the
host surface. This is the accepted price of one version and is the safer default
(any release could change host semantics); it is recorded in the design so it is
not rediscovered as a bug.
