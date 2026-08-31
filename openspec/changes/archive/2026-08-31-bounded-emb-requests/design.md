## Context

Current state: `handleEMB` accepts `len(cmd.Args)-2` texts with no cap; `handleEMBMULTI` accepts unlimited pairs and fans out to ≤ GOMAXPROCS goroutines (`multiPairFanOut`), each pair a 1-text `Pool.Embed`. The batcher's 16384-token window (`max_batch_tokens`) bounds *windows*, but a single request is never split or bounded (`token-budget-batching`). `max_concurrent_requests` gates in-flight *count* — it cannot bound the size of one command. The gem composes unbounded `EMB.MULTI`s in `BATCH_BLOCK` (`gems/emb/lib/emb/batch.rb`) and `MultiProxy#run` (`multi.rb`); `read_timeout` defaults to unset (redis-client ≈5s) while a giant MULTI runs for minutes. Runtime config exists as a `configParam` registry (`internal/server/config.go`) editable via `CONFIG GET/SET`.

Observed failure signature: sustained all-core CPU on scattered instances with flat memory/disk/network, re-saturated replacements (queued commands never cleared), triggered by cold-start traffic. Production learnings that shape the design (see proposal): the lazy path (feature-flag gated in the consuming app) is the **necessary condition**; a service restart is the **trigger** because cold-start concurrency makes `batch-loader`'s per-thread executors (shared `Concurrent::Future` pool threads, SQS/Sidekiq worker threads — never cleared by request middleware) accumulate burst-sized sets that fire as giant `EMB.MULTI`s on first use; steady state drains the sets continuously (small batches, healthy fleet). Amplifiers: read-timeout redelivery, small client pools blocking on checkout, and un-purged queues re-sticking replacements.

## Goals / Non-Goals

**Goals:**
- No single client command can monopolize a task's inference for unbounded time — work per command is capped by texts/pairs with a sane default.
- Never hard-fail a client: oversized commands are **truncated** (first N processed, overflow → `null`), so availability is preserved for every client, gem or not.
- Make the truncation loud and tunable: nulls in the reply (observable through the existing MGET nil path), `truncated_texts`/`truncated_pairs` counters, and runtime config (no restart to change caps).
- The gem never composes an oversized `EMB.MULTI`; each chunk fits comfortably under a typical read timeout so timeouts-and-redelivery can't re-amplify.
- Zero change to success-path wire formats (`EMB` reply, `EMB.MULTI` ordered array, MGET nil semantics).

**Non-Goals:**
- No hard rejection of oversized commands (availability preference: truncate, never reject).
- No server-side chunking/splitting of oversized commands (see Decision 2), no token-count admission (Decision 1), no per-request timeout/cancellation of an in-flight `session.Run` (ORT has no cheap abort; bounded admission is the v1 control).
- No redcon-level command-size cap (parse cost of an already-sent giant command is bounded by memory, not by these caps — see Risks; a `read-buffer`/max-command-size limit is a follow-up).
- No change to `max_concurrent_requests` semantics, batching window policy, or the 16384 budget.

## Decisions

### 1. Count-based caps (texts/pairs), not token-based

`max_texts` / `max_pairs` gate at admission with zero pre-tokenization cost. A token cap would require tokenizing every command twice (once at admission, once in the batcher) and is model-dependent for MULTI (mixed `max_length`s across models). The effective per-command token bound is `cap × model max_length` — for defaults and `max_length ≤ 512`, ≈2M tokens ceiling, which is the safety valve, not the operating point: the gem's `batch_size` (Decision 7) keeps real commands 1–2 orders of magnitude under it.

### 2. Truncate oversized commands; do not reject, do not chunk server-side

The server processes the first `max_texts`/`max_pairs` items and returns `null` for the overflow — MGET-style — so **any client keeps working** and the request is never a hard failure. Truncation trivially bounds work: a 100k-pair MULTI infers at most `max_pairs` pairs.

Why truncate instead of reject: rejection forces the *producer* to adapt and converts client timeouts into hard errors during rollout; truncation keeps the cargo moving for every client and makes overflow visible through the protocol (nulls) plus counters. Nulls compose safely with the gem (`entry&.unpack('e*')` → `nil`; a multi-text `EMB` reply keeps one slot per text, so callers cannot misalign). The reply array SHALL have one slot per requested item — never a shortened array — because a shortened array is the one shape that silently misaligns batch callers.

Server-side *chunking* remains out of scope: chunking does not reduce total work (every pair's tokens must still compute), it only changes pacing. Truncation is the work bound; chunking is the client's job.

**Alternative considered (rejected):** hard rejection with instructive error. Kept as a possible follow-up for operators who prefer fail-loud; truncation's nulls + counters cover observability without the deployment coupling.

### 3. Defaults: 4096 texts/pairs; `0` = unlimited

Unset defaults to 4096 (was: unlimited) — invisible to the observed healthy traffic (single-item commands), safely above sane bulk use, and below the ~10k+ pair scopes the batch-loader can compose after a cold start. `0` = unlimited matches existing `max_connections`/`max_concurrent_requests` semantics for operators who want pre-change behavior. Both are runtime-settable via `CONFIG SET`, so operators can tune during rollout without restarts.

### 4. Reply shape: one slot per item, overflow is null

`EMB` with N texts and cap M (N > M): reply is an array of N entries — M embeddings, N−M nulls. `EMB.MULTI` with N pairs: reply is an array of N entries — M embeddings (or per-pair nils from failures within the prefix), N−M nulls. The single-text `EMB` bulk reply is unchanged (a 1-text command cannot exceed a positive cap). This keeps every existing client's offset-based result mapping correct.

### 5. Truncation counters are global, in EMB.STATS

`truncated_texts` and `truncated_pairs` in `EMB.STATS` (flat fields, same style as `active_requests`), counting overflow items dropped (not the number of commands). MULTI is cross-model, so per-model attribution is meaningless for pairs; per-model counters in `EMB.INFO <model>` for the EMB path were considered and deferred (one global signal is enough to detect and tune; keep the surface minimal). The batcher's existing `requests`/`tokens` counters double as the quantitative bound proof (Decision 6).

### 6. Config surface: YAML + flags + CONFIG registry

`max_texts`/`max_pairs` in `Config`, parsed like `max_connections` (`internal/config/config.go` + `ParseFlags`), then wired into `Server` fields and the `configParam` registry (`internal/server/config.go`, alongside `cache_file`/`cache_save` pattern) so `CONFIG GET max_*` and `CONFIG SET max_*` work live. Non-integer values → startup error (config) / CONFIG SET error (runtime).

### 7. Gem chunks at `batch_size` (default 512), always

Both composition paths split pairs into `each_slice(batch_size)` sub-batches and issue one `EMB.MULTI` per slice, reassembling by offset:

```ruby
# batch.rb BATCH_BLOCK, per client, chunked:
client_items.each_slice(batch_size) do |slice|
  pairs = slice.flat_map { |_, model, text| ... }
  results = Array(client.send_command('EMB.MULTI', *pairs))
  # offset accounting within the slice, loader.call per item, MGET nil preserved
end
```

`batch_size` is a new `Configuration` option (global via `Emb.configure`, per-client override), default 512. Chunking is unconditional (not error-triggered) so commands stay small even against a misconfigured server and single-command duration stays under typical read timeouts. `EMB.STATS` pair-counting behavior is unchanged — the server still counts pairs, chunked or not.

**Why chunking (not clearing) is the right gem-side control**: the accumulation happens on threads the application never clears (shared `Concurrent::Future` pool threads, SQS/Sidekiq workers) and in defer-then-use scopes; you cannot reliably clear those from the gem. Limiting the *command size* bounds the damage for every accumulation shape and makes the residual behavior uniform (many small commands instead of one giant), which also keeps command duration under client read timeouts so the timeout-redelivery amplifier cannot form.

### 8. Deployment: server can ship first; gem chunks for quality

Because truncation never hard-fails a client, the server change is safe to roll out before the gem change. Until the gem chunks, oversized scopes get null-laden replies (visible, not silent) — the counters surface it. The gem chunking removes truncation from the normal path entirely.

## Risks / Trade-offs

- **Truncated output is data loss, by design**: overflow items return `null`. Mitigations: one-slot-per-item replies (no misalignment), nulls observable through the existing gem failure path, `truncated_*` counters, and gem chunking (default 512 ≪ 4096 cap) so the normal path never truncates. Non-gem clients must treat `null` reply entries as per-pair MGET failures — the same contract `EMB.MULTI` already has.
- **Already-queued giant commands**: caps truncate inference work but do not shrink already-queued commands' parse cost or remove them from a queue; producers still need client-side chunking or drop/park oversized jobs at the queue layer. Not a code fix.
- **Truncation parses before truncating**: a 100MB command still costs full parse CPU/memory before the cap check (redcon parses all args first). Bounded by memory, and the gem stops *sending* giant commands — but a hostile/misconfigured client can still send absurd commands. Follow-up: redcon-level max command size.
- **A client that depends on receiving every embedding gets nulls beyond the cap**: caps are runtime-tunable (`CONFIG SET max_*`), defaults are documented, and sizing guidance ties `batch_size` to the caps.
- **More round trips for large batch scopes**: N_chunks commands instead of 1. Acceptable — the pre-restart steady state (small scopes → 1 command) is unchanged; only pathological scopes change, and they get *bounded latency* instead of timeouts.
- **Tuning drift**: `batch_size` (client) and `max_texts`/`max_pairs` (server) are decoupled knobs; sizing guidance ties them ("chunk size should keep a command well under the cap and under the read timeout"). `CONFIG GET max_*` + `EMB.STATS truncated_*` make drift observable.
- **INT8/SigLIP asymmetry**: effective token ceiling differs per model (`max_length` 64 vs 512); count caps are model-agnostic by design; close enough for a safety valve.

## Migration Plan

- Land server caps first (safe alone — truncation never fails clients), then gem chunking (removes null-laden replies from the normal path). Defaults (4096 / 512) are invisible to normal traffic.
- Operators: size the caps against known legitimate command sizes; tune live with `CONFIG SET`. Park or drop oversized queued commands at the queue layer during rollout if present. Pre-fix stopgap while saturated: `CONFIG SET max_concurrent_requests N` bounds concurrent in-flight commands (fail-fast busy errors break the timeout-redelivery loop) but does not shrink a single running giant — combine with queue purge + task restart.
- Feature-flag rollout discipline for the consuming app: the lazy path concentrates all embedding compute on the fleet at 100% rollout; ramp with fleet capacity and treat full rollouts as capacity events.
- Rollback: `CONFIG SET max_texts 0 max_pairs 0` server-side (unlimited, pre-change behavior); gem rolls back to single-MULTI composition.