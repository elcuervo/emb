## Why

`lazy: :batch` exists to fan stateless work out concurrently, but today it packs a mixed-model scope into a single serial `EMB.MULTI` and only parallelizes when a scope exceeds `batch_size` texts. So a typical request (e.g. one `siglip2` text + one `hyperclusters` text) runs as one sequential `EMB.MULTI siglip2 <q> hyperclusters <q>` — the same wire behavior as `:multi`, and no latency win. Script calls (`eval`/`evalsha`) ignore the lazy mode entirely and always block.

The bug originates in the archived change `2026-09-04-lazy-execution-modes`: its proposal promised "a 60ms and a 10ms share complete in ~60ms", but its design (decision 3: "shares = ceil(items / batch_size)", "or `EMB.MULTI <model> <text>...` when the share spans models") and spec delta (`ruby-batch-loading` scenario "Mixed-model loaders coalesce into one MULTI … under a deferred mode"; `client-multi-instance-distribution` "pairs spanning two chunks") defined shares by text count instead of by independent command. Under that definition a small mixed-model scope is one share and can never run in parallel.

## What Changes

- `:multi` keeps its meaning: defer, coalesce into `EMB` (one model) or `EMB.MULTI` (mixed), send serially.
- `:batch` SHALL NOT send `EMB.MULTI`. A resolving scope is split into one plain `EMB <model> <text>...` share per model (chunked by `batch_size` within a model), and all shares are dispatched concurrently across instances and pool connections — regardless of scope size. Emb is stateless, so any connection can answer any share.
- `eval`/`evalsha` (`EMB.EVAL`/`EMB.EVSHA`) respect the lazy mode: eager under `false`; deferred under `:multi` (sent one after another on force) and `:batch` (each call is its own share, dispatched concurrently with the `EMB` shares of the same scope). `decode:` applies when the value resolves. **BREAKING** for callers that set a deferred mode and expect `eval` to block and return a plain value immediately.
- README "Lazy batching" section corrected to describe per-model / per-script shares.

## Capabilities

### New Capabilities

### Modified Capabilities
- `ruby-batch-loading`: mixed-model `EMB.MULTI` coalescing scoped to `:multi`; `:batch` defines shares per model (and per script call) and dispatches them concurrently even for small scopes; scripts join the deferred scope.
- `client-multi-instance-distribution`: concurrent fan-out dispatches per-model/per-script shares, not text-count chunks.
- `emb-ruby-client`: `eval`/`evalsha` honor the configured `lazy` mode.

## Impact

- `gems/emb/lib/emb/batch.rb`, `batch_dispatch.rb` (share packing and dispatch selection), `commands.rb` / `emb.rb` (`eval`, `evalsha`).
- `gems/emb/spec/emb_batch_spec.rb` and script specs; README.
- Consumers on `lazy: :batch` (unsplash-api `Phrase` defers `siglip2` + `hyperclusters`) get two concurrent `EMB` commands instead of one `EMB.MULTI`. No server changes.
