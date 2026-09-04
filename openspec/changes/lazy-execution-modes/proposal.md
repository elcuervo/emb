## Why

The emb Ruby client takes a binary stance today: `batch: true` (default) makes every embed call lazy and coalescing, and there is no way to execute the coalesced work in parallel. With multiple emb instances behind a pool, the client still serializes chunk after chunk, so a batch whose pieces take 60ms and 10ms costs ~70ms instead of the ~60ms it would cost when executed concurrently. We want a deliberate execution-mode model: an honest eager default, a coalescing mode (`:multi`), and a parallel fan-out mode (`:batch`) that distributes work across all configured emb instances — the deployment already runs several.

## What Changes

- **BREAKING** Replace the `batch: true/false` configuration option with a tri-state `lazy: false | :multi | :batch` (default `false`). The three values are mutually exclusive by construction; `:multi` means "defer and coalesce into `EMB.MULTI` chunks, serial execution" (today's batch behavior), `:batch` means "defer and execute chunks concurrently".
- **BREAKING** Flip the default to eager: with `lazy: false`, `Emb[:model][text]` SHALL send an immediate `EMB` command (one round trip per call, no coalescing machinery active).
- `url` SHALL accept either a String or an Array of Strings: multiple emb instances (the cluster topology), no separate `cluster:` key.
- With multiple urls, work distributes across instances: eager calls round-robin at command granularity, `:batch` mode fans out chunk shares in parallel, one per instance.
- `:batch` without multiple urls SHALL still execute concurrently over the client's pool connections to the single instance (client-level parallelism; the server's batching window may coalesce the concurrent chunks — accepted trade).
- `Emb.multi { }` SHALL remain the eager shape API in every mode (compose pairs into chunked `EMB.MULTI`, sent at block end).
- **BREAKING** Remove the explicit `Emb.batch` / `client.batch` lazy proxy entry point; the mode config governs deferred behavior, and `Emb.multi { }` covers explicit composition. (Per-call escape hatches can be revisited later if a real need emerges.)
- Failure semantics preserved and extended: a read-timeout after send stays fail-closed (the command may have executed; no duplicate inference), while a pre-send connection failure on one instance SHALL retry that chunk on another instance (safe — nothing was sent).
- Scope-tearing middleware (Rack/job/Rails) SHALL be inert in eager mode and active only when a deferred mode is configured.

## Capabilities

### New Capabilities
- `client-multi-instance-distribution`: multiple `url` values; instance-level round-robin distribution for eager calls; concurrent chunk distribution for `:batch`; per-instance connection pools; pre-send failure retry on another instance; result ordering preserved.

### Modified Capabilities
- `ruby-batch-loading`: configuration domain changes from `batch` to the `lazy` enum (defaults flip to eager); the `:batch` mode adds concurrent chunk execution; the explicit `Emb.batch` proxy API is removed.
- `emb-ruby-client`: default proxy behavior becomes eager (`lazy: false`); `url` accepts an array; `Emb.multi { }` explicitly remains eager in all modes.
- `client-global-configuration`: configuration surface changes (`batch` → `lazy`, `url` may be an array) and out-of-the-box defaults switch to eager single-command behavior.
- `ruby-client-round-robin`: nails down the two distribution axes — instance-level across urls, connection-level within each instance — and how they compose.

## Impact

- Ruby client gem: `gems/emb/lib/emb/{configuration,client,proxy,batch,multi,round_robin_pool,batch_scope,middleware,job_middleware,railtie}.rb` and their specs; `gems/emb/lib/emb.rb` module surface.
- Server: no changes — the server already bakes concurrently arriving requests into its batching window; the client change consumes that capability.
- Docs/specs: `gems/emb/README*`, openspec capability specs listed above; benchmark harness unaffected (server-side command behavior unchanged).