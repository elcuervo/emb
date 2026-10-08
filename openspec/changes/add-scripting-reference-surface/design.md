# Design

## Decision 1 — the scripting surface is its own Read surface

**Context.** Scripting already has partial homes: the landing's scripts block
(Persuade), the demos plate "the model is a function" (Experience/Read), and the
docs surface's `05 · Lua scripting` (Read). The request is a new *part* carrying
**all** of the engine's properties.

**Options.**

| | New `/scripting/` surface | Expand docs `#scripts` | New demo plate |
|---|---|---|---|
| Carries the full API | yes, room to grow | yes, but the docs page becomes a manual inside a manual | no — the gallery teaches concepts |
| One home for the fact | clean if docs links out | cleanest, at the cost of a 1400-line docs page | adds a fourth partial home |
| Nav cost | new page, four nav bars, `published-tree.py` | none | gallery index, `published-tree.py` |

**Decision.** A new Read surface at `website/scripting/`. It is a peer of Docs
and Demos, so it joins the primary nav. The docs surface keeps the narrative and
links here for the complete form, satisfying `product-docs`'s "a single fact has
one home" rule.

## Decision 2 — one version, sourced from `VERSION`

**Context.** `internal/script/version.go` carried `const APIVersion = "1.3.0"`
with a 1.0.0→1.3.0 history, exposed as `emb.API_VERSION` and folded into the
reply-cache key. The server already carries the build version from `VERSION`.

**Decision.** The scripted surface reports the server version.

```
VERSION ──ldflags -X main.version──▶ main.version
        ──Server.SetVersion────────▶ s.version
                                   └▶ script.APIVersion   (emb.API_VERSION + cache key)
```

`script.APIVersion` is a package variable (it comes from the build, not from
source), written once by `script.SetVersion` during startup before any
connection is served; `DefaultVersion = "dev"` covers `go test`/`go run`. An
empty injected version keeps `dev`, so the scripted surface and `INFO`'s
`emb_version` can never disagree. No second ldflag, no generated file.

**Rejected.** A second `-X` target on `internal/script` — that is two build
versions, the fragmentation this removes. A `go:generate`d file — machinery for
a value `VERSION` already threads.

## Decision 3 — the version folds into the reply-cache key, including its cost

The value is folded into `sha256(keyVersion|representation|apiVersion|…)` exactly
as the 1.x host API was. Consequence: **every upgrade invalidates cached script
replies**, including performance- and docs-only releases. Accepted: it is the
safer default (any release could change host semantics), and it is the direct
cost of one version. The `cacheKey` helper still takes the version explicitly, so
the folding is unit-testable at a fixed version.

The `script-eval` "Script API version" requirement is rewritten to match: the
value is the server version; the old "stable across performance work" clause is
removed because release-versioning contradicts it.

## Decision 4 — no reconstructed 1.x→emb history

The old host-API history (1.0.0→`v0.4.0`, 1.1.0→`v0.4.0`, 1.2.0→`v0.4.0`,
1.3.0→`v0.4.1`) could be reconstructed from git, but the maintainer chose to drop
it. The reference page therefore makes no per-feature availability claim against
old versions; it describes the current surface, and `emb.API_VERSION` reports the
server version a script is talking to.

## Decision 5 — inheritance, not a new world

The surface is **Read** and inherits everything: `styles.css` tokens and atoms,
the masthead and footer unchanged, the `docs.css` primitives
(`.doc-head`, `.doc-toc`, `.doc-body`, `.doc-h3`, `.doc-cmds`, `.doc-note`,
`.doc-prose`, `.code`, `.block--paper`/`.block--dark`), the 12px/14px type
floors, the 44px target floor, the reduced-motion and print behavior. It adds no
stylesheet, no colour, no font, and no component language. A surface brief lands
at `website/.impeccable/surfaces/website-scripting-index-html.md`.

The page reads with JavaScript off. The only scripting is the shared `tldr.js`
(the Read surfaces' TL;DR switch) — the page's `.tldr` lines are its static
summary and the long form paints without it.

## Decision 6 — the page's content contract

Sections, in the order a reader asks:

1. **The script surface** — `model(fn(input)) → output`, `KEYS`/`ARGV`, one
   evaluation per request, one return per text.
2. **Commands** — `EMB.EVAL`, `EMB.EVSHA`, `EMB.SCRIPT LOAD|EXISTS|FLUSH`.
3. **Preloading from config** — the `scripts:` entry shapes and `emb.script.config`.
4. **The sandbox** — what is stripped, fresh state, determinism.
5. **The host API** — the complete `emb.*` and `json.*` set.
6. **Tensors and replies** — the input spec, dtypes, packed form, the reply grammar.
7. **Budgets** — every cap, with its value.
8. **Caching and identity** — the per-model SHA1, the key composition, the
   preloaded digests.

Every value on the page derives from the shipped code and specs, per
`product-site`'s "README is the source of truth" rule. The version is stamped
from `VERSION`.
