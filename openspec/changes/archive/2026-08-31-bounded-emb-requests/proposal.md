## Why

The `emb` Ruby client composes its embedding demand into **one unbounded `EMB.MULTI` per request scope**: `Emb.batch`'s `BATCH_BLOCK` coalesces every deferred pair into a single `EMB.MULTI`, and `client.multi` does the same via `MultiProxy#run`.

The server has **no admission caps**: `EMB` accepts unlimited texts (`len(cmd.Args)-2`), `EMB.MULTI` unlimited pairs. An oversized command blows past the 16384 `max_batch_tokens` window budget — and per the `token-budget-batching` spec an oversized single request is *never split*, so it runs whole as one giant inference, pinning all cores (`intra_op_threads` = cores−2 + tokenizer workers) for minutes.

The gem's effective read timeout (~5s, `read_timeout` unset) is far smaller than such a command's duration → clients time out and re-deliver the same giant command → retry pile-up → sustained saturation that outlives any single run, and freshly started instances pick up the same queued commands and saturate again.

### Causal model (learned from a production deployment)

The trigger is **not** raw load — the lazy batching path can run for a long time with small, healthy batches. The necessary condition and the trigger are distinct:

1. **Necessary condition — the app routes embeddings through lazy batching.** In the consuming application the embedding path is feature-flagged: when off, embeddings are computed in-process (eager); when on, every embedding call becomes a lazy `Emb[...]` batch-loader item. The fleet only becomes load-bearing at 100% of traffic.
2. **How batches grow — per-thread, fired on first use.** `batch-loader` registers items on `Thread.current`'s executor; a batch fires when a value is *used*, grabbing **everything pending on that thread** since its last flush. Request middleware (`clear_current`) only clears the **request thread**. Embedding calls that happen on shared `Concurrent::Future` pool threads, SQS/Sidekiq worker threads, or on batch scopes that defer-then-use (`list.map(&:embeddings)` then consume) accumulate across requests/jobs with no clearing — one thread's set can hold hundreds–thousands of pairs at its next sync.
3. **Trigger — cold start synchronizes the burst.** After a service restart, all processes miss their in-process/per-request caches at once; a burst of concurrent requests defers many loaders in a few seconds onto a small shared thread pool with empty-but-refilling executors; the first consumer-syncs emit `EMB.MULTI`s sized by **burst concurrency**, not steady demand. In steady state the sets are drained continuously by uses (small batches), which is why the same code is harmless until synchronized.
4. **Amplifiers — timeouts, small client pools, queued re-delivery.** Command duration (minutes) ≫ client read timeout (~5s) → disconnect-and-redeliver of the *same* giant command; a small client connection pool (3) makes request threads block on checkout, compounding timeouts; the queue keeps the giant jobs, so replacement instances immediately re-stick unless the queue is purged.

Fix both ends: **bound request sizes server-side by truncating** oversized `EMB`/`EMB.MULTI` commands to a configured cap (overflow returns MGET-style nulls — never a hard failure, availability preserved) and **chunk client-side** (the gem never composes an oversized `EMB.MULTI`; each chunk fits comfortably within read timeouts). Chunking bounds the damage for *any* accumulation shape — per-request scope, shared pool thread, or worker loop — because the weapon is always a too-large command.

## What Changes

- **Server caps truncate.** `max_texts` bounds inferred texts per `EMB`; `max_pairs` bounds inferred pairs per `EMB.MULTI`. The first N items are processed normally; overflow items are **not** tokenized, inferred, or cached, and their reply slots are `null` — the reply array keeps one slot per requested item, so clients can never misalign results. Truncation is the safety valve: any client keeps working; no hard errors.
- **The `token-budget-batching` "single-request batches are never split" guarantee is replaced**: an oversized command is truncated to the cap instead of running whole as one unbounded inference. Commands within the cap keep identical behavior (one run, all embeddings returned).
- **`EMB.MULTI` reply construction is bounded** by `max_pairs` (fan-out already bounded; the reply array and result slice now are too).
- **Observability.** `EMB.STATS` gains `truncated_texts` and `truncated_pairs` counters (overflow item counts) so operators can see clients hitting the caps without logs. The batcher's existing `tokens`/`requests` counters are the quantitative proof of bounded work (a 100k-pair MULTI contributes ≤ `max_pairs` requests and ≤ `max_pairs × max_length` tokens).
- **Gem chunking.** `Emb.batch`'s batch block and `client.multi` split composed pairs into ≤ `batch_size`-pair `EMB.MULTI`s (new config option, default 512). Result ordering and MGET-style per-pair nil semantics are preserved; a scope's pairs still resolve together, just in multiple commands. Truncation nulls flow through the existing `entry&.unpack('e*')` nil path, so an app that over-sends sees explicit `nil`s, not silent misalignment.
- **Docs.** config.yaml comments for the new keys; sizing guidance (keep `batch_size` well under the caps and command duration under the client read timeout) is documented in the change artifacts and CHANGELOG/release notes at release time.

## Capabilities

### New Capabilities

- `request-size-guardrails`: server-side caps for `EMB`/`EMB.MULTI` (`max_texts`, `max_pairs`) that truncate oversized commands, and truncation counters.

### Modified Capabilities

- `token-budget-batching`: oversized single-request behavior changes from "never split, run whole" to "truncated to the cap".
- `emb-multi`: pair-count cap bounds the command, overflow pairs return null.
- `ruby-batch-loading`: batch-loader composes chunked `EMB.MULTI`s instead of one unbounded command.
- `emb-ruby-client`: `client.multi` chunks; `batch_size` is a client config option.

## Impact

- **Code:** server — `handleEMB`/`handleEMBMULTI` truncation, `Server` config fields + `configParams` registry entries, `EMB.STATS` counters, config parsing/flags; gem — `batch.rb`, `multi.rb`, `configuration.rb` (`batch_size`); tests for all.
- **APIs:** no new error paths — oversized commands return success replies with `null` in overflow slots; `EMB.STATS` adds two fields; config surface additive (`max_texts`, `max_pairs`, `batch_size`). Success-path wire formats unchanged.
- **Systems:** server + gem client. Deployment is coupled for *quality* (gem chunking prevents null-laden replies) but truncation alone never breaks a client, so the server can ship first without an outage window.
- **Sequencing:** independent of `cache-snapshot`; `CONFIG SET` drives caps live, so operators can tune during rollout without restarts.
- **Operations companion:** caps truncate the inference a command performs but do not shrink already-queued commands or remove them from a queue; producers that re-deliver on timeout should chunk client-side or drop/park oversized jobs at the queue layer. Until this change ships, `CONFIG SET max_concurrent_requests N` is a partial stopgap: it caps *concurrent* in-flight commands (fail-fast `ERR busy` instead of timeout-redelivery) but does not shrink a single already-running giant command — queue purge + task restart remain the immediate remedy for queued giants.