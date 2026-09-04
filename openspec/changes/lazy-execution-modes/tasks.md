## 1. Configuration surface

- [x] 1.1 Replace `batch` with `lazy` (`false` / `:multi` / `:batch`, default `false`) in `Configuration` (`OPTIONS`, initializer) and validate the value; verify `Emb.new(lazy: :eager)` raises and `Emb.new(lazy: :multi)`/`:batch`/`false` accept (unit spec)
- [x] 1.2 Accept `url` as String or Array in configuration and client construction; normalize to an array internally; verify an empty array raises a clear error (unit spec)
- [x] 1.3 Remove `Emb.batch` / `client.batch` and the `BatchProxy`/`BatchModelProxy` classes; verify `Emb.batch` raises `NoMethodError` and `client.batch` is gone (unit spec)

## 2. Multi-instance client

- [x] 2.1 Build per-instance `RoundRobinPool`s in `Client` (one per url, each sized `pool`) and add instance-level round-robin selection; verify 3 urls × pool 2 rotate instances first, connections second, with `PING` (unit spec)
- [x] 2.2 Route `send_command` through instance selection → connection; verify eager `EMB` calls distribute across instances in rotation order (unit spec, mocked/socket-level)
- [x] 2.3 Verify fork handling still resets every per-instance pool (existing `reload_after_fork!` coverage extended to multiple pools; unit spec)

## 3. Lazy mode behavior

- [x] 3.1 Make the standard proxy (`Proxy#[]`) eager by default: sends `EMB` immediately when `lazy: false`; verify default `Emb[:minilm]["hello"]` returns Array of Float without deferral (unit spec, no batch scope active)
- [x] 3.2 Defer proxy calls under `lazy: :multi` and `lazy: :batch` exactly as the old `batch: true` path did; verify no command at call time and `EMB.MULTI` on use, values equal to eager results (unit spec)
- [x] 3.3 Keep per-scope coalescing, chunking at `batch_size`, MGET-style per-pair nil, scope caching/dedup, and fail-closed resolution under both deferred modes; verify with the existing coalescing/dedup/failure scenarios updated to mode-driven proxies (unit specs)

## 4. Parallel batch execution

- [x] 4.1 Rework `BATCH_BLOCK` to dispatch chunk shares concurrently (worker thread per share, join, reassemble in slice order, errors re-raised on the forcing thread); verify a multi-chunk scope under `lazy: :batch` overlaps in flight and returns results in deferral order (timing-free threaded unit spec)
- [x] 4.2 Implement pre-send failure retry for a share (retry on another instance, `ConnectionError` only; never re-dispatch after `TimeoutError`); verify a dead instance's share completes via a healthy instance and a timeout is surfaced without re-send (unit spec with simulated instance failure)
- [x] 4.3 Verify terminal share failure follows fail-closed semantics: force raises once, successful shares are not re-sent, failed items clear from the pending set (unit spec)

## 5. Middleware and module surface

- [x] 5.1 Make `Middleware`, `JobMiddleware`, and `BatchScope` inert when the mode is `lazy: false` (no scope manipulation); verify default-mode middleware passes through without clearing and deferred modes still clear scopes (unit specs)
- [x] 5.2 Update `emb.rb` module surface (remove `batch`, keep `multi`, `[]`, etc.) and the railtie wiring; verify module-level entry points against a client with `url` array still work (integration spec)

## 6. Spec consolidation

- [x] 6.1 Update gem unit/integration specs and README to the `lazy` enum, eager default, `Emb.multi` eager across modes, and multi-instance `url` arrays; verify `cd gems/emb && bundle exec rake` passes against a running server on :16379 with a two-model config
- [x] 6.2 Run `cd gems/emb && bundle exec rubocop` and the full repo checks (`just test`, `just lint`); verify clean