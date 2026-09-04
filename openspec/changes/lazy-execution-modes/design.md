## Context

The emb Ruby client (`gems/emb`) today wires laziness through `batch: true` (default) via the `batch-loader` gem: `Proxy#[]` returns a deferred `BatchLoader`, and the shared `BATCH_BLOCK` in `batch.rb` groups deferred items by client object, chunks them at `batch_size` pairs, and sends `EMB.MULTI` commands **serially** (one `each_slice` after another). `Emb.multi { }` (`multi.rb`) is an eager compose API that sends the same chunked `EMB.MULTI` shape. Distribution across instances is indirect: `RoundRobinPool` rotates N connections to a single URL, and a connection-level LB (Service Connect / NLB) pins each connection to one upstream instance.

The change: a `lazy` mode enum (`false` / `:multi` / `:batch`, default `false`) replaces the `batch` boolean; `url` accepts an array of instances with per-instance pools; `:batch` mode dispatches chunk shares concurrently (across instances when several are configured, across a node's pool connections otherwise). Specs: `lazy-execution-modes` change; delta specs in `ruby-batch-loading`, `emb-ruby-client`, `client-global-configuration`, `ruby-client-round-robin`, and the new `client-multi-instance-distribution`.

## Goals / Non-Goals

**Goals:**
- `lazy: false | :multi | :batch` configuration with eager default; invalid values rejected.
- `url` as String or Array of Strings; one pool per instance; instance-level round-robin.
- `:batch` executes chunk shares concurrently; results reassembled in deferral order.
- Pre-send connection failures retry the share on another instance; read timeouts never re-dispatch.
- `Emb.multi { }` stays eager in every mode.
- Explicit `Emb.batch` / `client.batch` API removed.
- Scope-tearing middleware inert in eager mode.

**Non-Goals:**
- Server-side changes of any kind (the server already coalesces concurrent EMB.MULTIs in its 1ms window).
- Instance discovery / health checks / re-resolution (static url list only).
- Smarter distribution strategies (least-loaded, affinity, power-of-two-choices); plain round-robin only.
- `lazy` mode changes at runtime (per-call mode overrides).

## Decisions

### 1. `lazy` mode enum replaces `batch` boolean

`Configuration` gains `lazy` with allowed values `false`, `:multi`, `:batch`, default `false`; `batch` is removed from `OPTIONS`. Client construction validates the mode (alongside pool/url validation) and raises `ArgumentError` for anything else. `Client` stores `@lazy_mode` and replaces `batch?` predicates: a proxy call defers iff `@lazy_mode != false`.

**Alternative considered**: keep `batch: true/false` and add a separate `parallel:` flag — rejected: two booleans can express invalid states (`batch: false, parallel: true`) and the proposal's convention is an exclusive enum.

### 2. `url` array → per-instance pools inside the one Client

`Client` keeps a single public identity but internally holds an ordered array of `RoundRobinPool`s (one per url). Instance selection is a lightweight atomic counter over the array (same pattern `RoundRobinPool#pick` already uses); within an instance, the existing `RoundRobinPool#with` rotates connections. `send_command` selects an instance, then a connection. This makes the "cluster" an invisible property of the client: proxy, multi, and command-wrapper surfaces are unchanged, matching the decision to fold the cluster into `url`.

Note: `RoundRobinPool` already registers every pool in `INSTANCES` for fork handling; per-instance pools register automatically, so `after_fork` keeps working unchanged.

### 3. `:batch` concurrency lives in `BATCH_BLOCK`

The lazy path stays on `batch-loader`; only the shared block changes. After grouping by client and slicing into chunks, `BATCH_BLOCK` dispatches the chunks concurrently:

- Spawn one worker thread per chunk share (bounded: shares = ceil(items / batch_size), so small in practice).
- Each worker calls `client.send_command` with the share's command: plain `EMB <model> <text>...` when the share's items share one model (the server packs the texts into one inference; `EMB.MULTI` would repeat the model per text and add pair fan-out for nothing), or `EMB.MULTI <model> <text>...` when the share spans models (preserving per-pair nil semantics). The router selects an instance/connection per command.
- The block joins all workers, assembles results in slice order, and resolves loaders in deferral order.
- Worker threads never touch `BatchLoader::Executor` (it is per-thread); they only perform I/O and return raw results. Errors are collected and re-raised on the forcing thread so `clear_batch_pending!` runs against the correct executor.

For single-instance clients this still yields concurrency: shares run in parallel across the instance's pool connections (the server's batching window may coalesce them; accepted per spec).

### 4. Retry: pre-send failures only, bounded

Per share: attempts = up to (number of instances) passes. Only `RedisClient::ConnectionError` family (connection never established / reset before write) triggers a retry on a different instance. `RedisClient::TimeoutError` (command sent, reply not read in time) is treated as terminal: may have executed, no re-dispatch anywhere. After retries are exhausted the share fails, the force raises on the forcing thread, and the share's items clear from the pending set (existing fail-closed semantics).

Distinguishing the two error classes is a deliberate, explicit contract: retrying on timeout would duplicate inference.

### 5. `Emb.multi { }` untouched, explicitly eager

`multi.rb` already sends at block end; no mode-dependent behavior is added. The `Emb.multi` and `client.multi` entry points keep sending chunked `EMB.MULTI` eagerly in every mode. The only edit is documentation/tests asserting this.

### 6. Middleware inert in eager mode

`Middleware`, `JobMiddleware`, `BatchScope`, and the railtie already guard on serializer state; they must early-return (no-op) when the client's mode is `lazy: false`. The per-thread scope mechanism is only meaningful for deferred modes.

### 7. Explicit `Emb.batch` / `client.batch` removed

`Client#batch`, `BatchProxy`, `BatchModelProxy`, and the module-level `Emb.batch` are deleted; module-level `emb.rb` no longer wires `batch`. `Emb.multi` remains the explicit composition tool. Specs migrated the old scenarios to the mode-driven proxy.

## Risks / Trade-offs

- **GVL**: Ruby threads share the GVL, but blocking socket I/O releases it, so parallel `send_command` genuinely overlaps; CPU-bound unpacking inside workers is small relative to network/inference time. Worker-per-share is cheap at realistic chunk counts (≪ 100).
- **Server coalescing on a single instance**: concurrent shares to one node may merge into one larger ONNX run (throughput engine), so wall-time gain on a single instance is workload-dependent — the decisive latency win needs multiple urls. Documented in the spec (single-instance concurrency is still guaranteed at the client level).
- **Ordering contract**: results are reassembled by slice index, so deferral order is preserved even though commands complete out of order. This is the one place a regression would be silent (no error, wrong pairing) — needs the dedicated threaded test.
- **Failure translation**: a mid-dispatch failure must not leak partial state into the batch-loader executor; keeping cleanup on the forcing thread (decision 3) makes this deterministic.
- **Dead instances**: with `reconnect_attempts: 0`, a dead instance's connections stay unavailable; pre-send retry routes around it per batch, and instance-level round-robin naturally shifts subsequent commands away. No health-checking in scope.
- **Breaking change**: the eager default changes out-of-the-box behavior (one round trip per embed call); `batch: true` configs must move to `lazy: :multi`.