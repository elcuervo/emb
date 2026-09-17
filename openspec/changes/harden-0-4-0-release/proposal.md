## Why

A pre-release review of `emb` and `gems/emb` found six defects that survive the current test suite: unsynchronized runtime config (`-race` reports), a stale `CONFIG GET cache`, a script reply-cache key that collides across call arity (returns wrong values), a Ruby `ready?` that is always true, a Ruby `protocol: 3` option that silently corrupts `models`/`stats`/`info`/`config`, and a cache that retains whole batch buffers through sub-slice aliasing. These must be fixed before 0.4.0 ships.

## What Changes

- **Go — concurrency-safe runtime caps.** `max_texts`, `max_pairs`, `max_images`, `max_image_bytes`, `max_image_pixels`, and `max_command_bytes` become atomic, so `CONFIG SET` and request handlers no longer race (`go test -race` clean).
- **Go — `CONFIG GET cache` echoes the live value.** `CONFIG SET cache <size>` updates the reported value, not only the byte budget.
- **Go — script reply cache is arity-scoped.** The content-addressed key includes the text count, so a single-text call and a multi-text call with the same text no longer collide and replay a wrong/erroneous reply.
- **Go — cached embeddings are owned copies.** The cache stores a copy of each embedding row, so a value never aliases a shared batch buffer and byte accounting matches retained memory.
- **Ruby — `ready?` is a real predicate.** It returns `false` when the server is not ready and on connection failure, instead of always `true` (or raising). `ready` becomes total: it returns the server's `OK`/error text and never raises.
- **Ruby — `protocol` is validated.** Only RESP2 (`protocol: 2`) is supported; any other value raises `ArgumentError` at configuration or client construction time instead of silently producing wrong replies. **BREAKING** for anyone who set `protocol: 3` (which never worked for the introspection commands).
- **Ruby — embed replies tolerate null slots.** A `nil` reply slot maps to `nil` instead of raising `NoMethodError`.
- **Examples / docs.** Fix the shipped `examples/scripts/snippets/qa.lua`, which read `KEYS[2]` but is documented with a one-text invocation, and correct `examples/scripts/README.md` to state the reply cache's per-text independence assumption.
- **Spec repair.** `openspec/specs/{script-eval,lru-cache}/spec.md` were structurally invalid (delta headers overwritten into the main specs by an earlier archive); restore and merge them so the specs parse and archive.
- **Packaging — `gems/emb/LICENSE`.** Add the MIT license file the gemspec already lists.

## Capabilities

### New Capabilities
<!-- none: every fix lands in an existing capability -->

### Modified Capabilities
- `redis-config-command`: runtime-editable parameters are safe to change while requests are in flight, and `CONFIG GET` reports the current value after `CONFIG SET`, including `cache`.
- `script-eval`: the script reply cache is scoped to the evaluation's text count, so cached replies are never replayed across a different call shape.
- `lru-cache`: values stored in the cache are independent copies and the reported memory reflects what the cache retains.
- `emb-ruby-client`: `ready?` reports readiness truthfully (`ready` is total), and embed replies tolerate null slots.
- `client-global-configuration`: the `protocol` option is restricted to RESP2 (2) and rejects any other value.
- `gem-redis-client-config`: the option-forwarding requirement no longer forwards `protocol` blindly; it is validated to RESP2.

## Impact

- `internal/server/server.go`, `internal/server/config.go`, `internal/server/script.go`, `internal/server/cache.go` — atomic caps, cache config echo, arity-scoped key, copy-on-set.
- `internal/script/cache.go` — key derivation signature.
- `gems/emb/lib/emb/{client,commands,configuration,runtime_config,proxy}.rb`, `gems/emb/LICENSE` — predicate, validation, nil tolerance, packaging.
- No server API or wire-format change. The script cache keys change, so previously cached script replies are simply a miss on first request after upgrade (cache is otherwise in-process).
