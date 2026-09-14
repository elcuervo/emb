## Why

`emb` ships a command surface, six example scripts, and a reference README — but
nothing in the repository shows the loop the product exists for: embed a corpus,
store the vectors, search them, get an answer. `examples/scripts/` demonstrates
*constructs*, one model at a time; a reader asking "what does a real application
look like" has to assemble it from the manual.

`emb` is an embedding **compute** server. That is its whole contract — compute
here, store and search somewhere else — and it is the most misread thing about
the project, because the repository states it only in prose. A runnable example
is the cheapest way to make the contract self-evident, and it doubles as the
end-to-end proof the product site currently only asserts.

Now is the moment because the other half became available at no cost: Redis 8's
`vectorset` module is already in the dev shell, and the Ruby client already
depends on `redis-client`, so both halves of the example speak through libraries
the repository already ships. Nothing needs to be added to make this possible.

## What Changes

- **Add `examples/kitchensink/`** — a runnable end-to-end vector application:
  - `app.rb` — `index FILE...`, `search QUERY [K]`, `stats`. Ingest, query,
    observe; no other surface.
  - `emb.yaml` — the example's emb instance, with the load-bearing fields
    present and the heavier tier as a commented block above it.
  - `README.md` — how to run it, what each half does, and the gotchas that
    silently degrade a vector index.
  - `run.sh` — start Redis, start `emb`, wait for `EMB.READY`, exec `app.rb`.
- **Add a `just kitchensink` passthrough** so the example is reachable from the
  repository's standard task surface.
- **Point `README.md` at the example** from its existing examples section.
- **No server, client, configuration, or protocol changes.** Every command the
  example uses already ships; the example is a composition, not a feature.

Deliberately not in this change: the website extraction. The site is a separate
surface with its own change in flight, its own design system, and a rule that
every rendered claim be traceable. The constructions worth extracting are named
in `design.md` so that change can be written against a settled example rather
than a moving one.

## Capabilities

### New Capabilities

- `kitchensink-example`: the example's contract — its file layout, the
  configuration fields that are load-bearing rather than stylistic, its command
  surface, the invariant that the indexed text is retrieved from the index
  itself rather than a second store, and the rule that every number its
  documentation prints is reproducible from a real run.

### Modified Capabilities

None. The example introduces no command, reply shape, configuration key, or
number the server does not already support, and it changes no existing
behavior — so no existing capability's requirements move.

## Impact

| | |
|---|---|
| New files | `examples/kitchensink/{app.rb,emb.yaml,README.md,run.sh}` |
| Modified files | `justfile` (one passthrough target), `README.md` (a pointer) |
| Runtime dependencies | None added. `redis-client` is already a dependency of `gems/emb`; Redis 8's `vectorset` module is already in the dev shell |
| Downloads | ~86 MB (minilm via `model_repo`), or nothing if `models/minilm` is present |
| CI / test surface | None. The example is not a test; `just test` and the gem suite are untouched |
| Risk | Contained to a new directory plus two additive edits |
