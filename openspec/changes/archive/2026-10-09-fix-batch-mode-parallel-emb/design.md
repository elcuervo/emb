## Context

`BATCH_BLOCK` (gems/emb/lib/emb/batch.rb) groups a scope's items by client, packs them by text count into slices of `batch_size` (`pack_slices`), and only calls `dispatch_parallel` when `slices.size > 1`. `dispatch_slice` sends `EMB` for a single-model slice and `EMB.MULTI` otherwise. So in `:batch` a small mixed-model scope is one slice → one serial `EMB.MULTI`, identical to `:multi`. `eval`/`evalsha` (commands.rb) call `send_command` directly and never defer.

This shape was specified by archived change `2026-09-04-lazy-execution-modes` (design decision 3 "shares = ceil(items / batch_size)", MULTI for model-spanning shares; spec scenario "Mixed-model loaders coalesce into one MULTI … under a deferred mode"), contradicting its own proposal's parallelism goal.

## Goals / Non-Goals

**Goals:**
- `:batch` never sends `EMB.MULTI`; it sends independent plain `EMB` shares concurrently, even for a 2-item scope.
- `eval`/`evalsha` defer under deferred modes and run as independent concurrent shares under `:batch`.
- `:multi` and eager wire behavior unchanged for embeds.

**Non-Goals:**
- Splitting one model's texts below `batch_size` across instances (the server already packs a single `EMB` into one inference; splitting trades batching efficiency for fan-out).
- Deferring `EMB.IMG` (`Proxy#image`) or the explicit `multi` block API.
- Server changes.

## Decisions

1. **Share = independent command, not text-count chunk.** In `:batch`, shares are built as: for each client → for each model (in first-deferral order) → `pack_slices(model_items, chunk)`; plus one share per deferred script item. Alternative (keep text-count packing, lower `batch_size`) rejected: forces callers to tune a knob and still mixes models.
2. **Always fan out when shares > 1.** Drop the `slices.size > 1` gate's dependency on chunk count — it now naturally triggers for mixed-model scopes. A single share runs on the forcing thread (no thread spawn), matching today's cost for the common one-model case.
3. **Scripts join the same BatchLoader scope.** Script items use a distinct item shape, e.g. `[client, :script, [cmd, model, script_or_sha, texts, args, decode], nil]`, under the same `BATCH_KEY`/`BATCH_BLOCK` so forcing an embed also resolves pending scripts and vice versa. `dispatch_slice`/`resolve_slice` branch on the `:script` marker: send the original `EMB.EVAL`/`EMB.EVSHA` args, then `parse_script_reply(reply, multi:, decode:)` on the forcing thread. `normalize_decode` runs at call time so bad `decode:` still raises immediately. Alternative (separate BatchLoader key for scripts) rejected: two keys = two executors flushes = no overlap between script and embed shares.
4. **`:multi` keeps scripts serial.** There is no MULTI form for scripts; under `:multi` script items are sent one at a time after the coalesced `EMB`/`EMB.MULTI` chunks.
5. **Failure semantics reuse fail-closed path.** A failed script share is a terminal share error like any other (`fail_batch!` with model context); successful shares are not re-sent. `default_value` for a failed script is `nil` rather than `[]`.

## Risks / Trade-offs

- [Deferred `eval` returns a BatchLoader proxy, not a Hash/Array] → only under an explicitly configured deferred mode; proxies delegate to the resolved value. Called out as BREAKING in README/CHANGELOG.
- [More connections in flight per request (one per model)] → bounded by `worker_capacity` (sum of pool sizes), as today.
- [Per-model `EMB` loses `EMB.MULTI` per-pair nil for unknown models] → under `:batch` an unknown model fails its own share; other shares still complete and are not re-sent. Callers wanting per-pair nil use `:multi`.

## Migration Plan

Gem minor bump. Consumers on `:batch` need no code change; wire traffic becomes N concurrent `EMB` commands. Rollback = previous gem version or `lazy: :multi`.

## Open Questions

- Should `Proxy#image` (`EMB.IMG`) also defer under deferred modes? Deferred to a follow-up.
