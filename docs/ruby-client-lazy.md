# The emb gem's lazy request operations

`Emb::Client.new(lazy:)` is the whole surface: one option with three values, and
nothing else about the client changes. Everything below is about *when* the
socket is used.

## The three modes

<img src="assets/diagrams/emb-gem-request-modes.svg" alt="The ruby emb gem has exactly one option, lazy, with three values, and nothing else changes. The default, lazy false, is eager: every client[:model][text] call sends its own EMB immediately and returns the decoded float array, so three calls cost three blocking round trips. lazy :multi defers: the same three calls return BatchLoader handles and nothing is sent. When any one of them is used as a value — calling sum, to_a or each forces it — all pending texts in that thread's scope are packed into shares of at most batch_size texts (512 by default) and sent one after another, serially. A share whose texts all name one model is sent as a plain EMB with the model once; a share with mixed models is sent as EMB.MULTI with per-pair null semantics. lazy :batch defers identically but dispatches the shares on parallel worker threads, capped by the total connection-pool size, so several requests are in flight at once. Deferral is scoped to the calling thread: the Rack middleware, the Sidekiq/Shoryuken/ActiveJob job middleware and an explicit BatchScope.wrap all clear the thread's batch scope in an ensure block, even when the work raises, so cached values and pending items cannot leak into the next request. A share that fails fails closed: the pending set is dropped and the caller gets Emb::ServerError with the models and text counts, and items with no value resolve to an empty array rather than nil.">


```
lazy: false   m['a'] ──▶ EMB minilm 'a' ──▶ Array<Float>     3 round trips,
              m['b'] ──▶ EMB minilm 'b' ──▶ Array<Float>     all blocking
              m['c'] ──▶ EMB minilm 'c' ──▶ Array<Float>

lazy: :multi  m['a'] ─┐
              m['b'] ─┼──▶ BatchLoader ──▶ force ──▶ EMB minilm a b c   serial slices,
              m['c'] ─┘                                            one at a time

lazy: :batch  m['a'] ─┐
              m['b'] ─┼──▶ BatchLoader ──▶ force ──▶ ┌ EMB minilm a..   slices run
              m['c'] ─┘                              ├ EMB.MULTI ...    concurrently,
                                                     └ EMB minilm z..   up to Σ pool sizes
```

The reply shape does not change between modes:

```ruby
client[:minilm]['hello']            # => [0.12, -0.04, ...]        one text, one vector
client[:minilm]['hello', 'world']   # => [[...], [...]]            many texts, many vectors
```

In a deferred mode those same calls return a `BatchLoader` handle that behaves
like the value once it is used. Deferral is a *per-thread* property, which is
why the middleware exists.

## The deferred batch lifecycle

<img src="assets/diagrams/emb-gem-lazy-batch-flow.svg" alt="Three stages. First, defer: calling client[:minilm]['hello'] on a lazy client does not touch a socket. Proxy#[] sees that the client is lazy and calls build_batch_loader, which wraps the tuple of client, model, text and format in BatchLoader.for and calls batch with default_value an empty array, the key :emb, and the shared BATCH_BLOCK lambda. That registers one item in the calling thread's ExecutorProxy and returns a handle; every further call on the same thread appends another item to the same scope, whatever model it names. Second, flush: the first value-consuming call on any handle — sum, to_a, each — goes through method_missing into __sync and __ensure_batched, which collects every pending item for this block and runs BATCH_BLOCK exactly once. The block groups items by client, packs them into slices of at most batch_size texts so a single command stays inside the server's pair cap and the client's read timeout, and creates slices no larger than the chunk even when one item alone exceeds it. dispatch_slice chooses the wire shape: a slice whose texts all name one model is sent as a plain EMB with the model named once, while a slice with mixed models becomes EMB.MULTI with one model-and-text pair per item, and a :values slice adds the VALUES keyword. resolve_slice maps the reply entries back onto items in deferral order and calls the loader once per item. On the parallel path the worker threads only capture outcomes; the forcing thread is the one that resolves loaders, so no loader is ever touched off its own thread. Third, settle: on success batch-loader deletes the batch's queued items and each handle caches its value, so later methods on it never re-send. On failure the gem calls clear_batch_pending! to drop the queue that batch-loader would otherwise retain, and a Redis error becomes Emb::ServerError carrying the models and text counts, while any item left without a value falls back to the empty-array default. ShortReplyError is raised locally, so it is never counted as a transport retry. Finally the per-thread scope is cleared in an ensure block by the Rack middleware, the Sidekiq, Shoryuken or ActiveJob job middleware, or an explicit BatchScope.wrap — without one, cached values and pending items would leak into the next request or job.">

The item tuple is `[client, model, text, format]`, and the batch block is keyed
by `[BATCH_BLOCK.source_location, :emb]`. That key is the mechanism behind
`clear_batch_pending!`: batch-loader prunes pending items only after a
*successful* batch, so the gem deletes them itself on failure — otherwise a
later batch in the same scope would re-send stale items.

`default_value: []` is what makes a failed embed resolve to an empty vector
collection rather than `nil`, so `loader.sum` raises nothing surprising.

## Packing, dispatch and the failure tail

<img src="assets/diagrams/emb-gem-batch-dispatch.svg" alt="A flush first packs items into slices. Texts are accumulated with a running count per slice until adding the next item would exceed the chunk size, which is the client's own batch_size or the global default of 512 texts; an item bigger than a whole chunk goes into a slice by itself and the server truncates it with null slots, exactly as the eager path does. Keeping a running count instead of re-summing the slice for every item is what keeps packing linear rather than quadratic. From there the two lazy modes diverge. On the serial path, lazy multi, the slices are dispatched one after another, a single share in flight at a time: each slice is shaped into wire arguments, sent, and its reply entries mapped back onto the items in deferral order. On the parallel path, lazy batch, the number of workers is the slice count clamped to the client's connection capacity, which is the sum of its pool sizes; each worker takes an indexed slice off a shared queue and records either an ok outcome or an error outcome, and workers never touch a loader. Only the forcing thread resolves loaders, walking the outcomes in index order and remembering the first error, so the ordering matches MGET semantics even when the shares ran concurrently. Both paths funnel a Redis error into one failure tail: the pending queue is dropped, the attempt count is the retry budget plus one only for errors redis-client actually re-dispatches — connection errors and protocol errors, never read timeouts or the gem's own short-reply error — and the caller gets Emb::ServerError naming the models and the text count, with the original error preserved as its cause. Anything that is not a Redis error is treated as a local bug and re-raised unchanged after the pending set is dropped. The VALUES format has its own resolution: a single-model slice replies with one envelope whose rows are re-sliced per item, a mixed-model slice gets one envelope per pair with nil for failures, and a reply shorter than expected raises ShortReplyError, which is terminal and counted as a single attempt.">

Two wire-shaping facts worth stating plainly:

- A slice whose texts all name **one** model is sent as `EMB <model> <text>...`
  with the model named once. Only a slice with **mixed** models becomes
  `EMB.MULTI <model> <text> ...`. The eager path never uses `EMB.MULTI` at all;
  `client.multi Ellipsis` does, explicitly.
- `:batch` does not mean "unbounded". The number of in-flight shares is capped
  by the client's total connection capacity, so a large batch cannot open more
  sockets than the pool owns.

## Connections, instances and retries

<img src="assets/diagrams/emb-gem-connection-router.svg" alt="Every command the gem sends goes through the client's send_command into a ConnectionRouter. The router owns one RoundRobinPool per configured url — the interchangeable emb instances that serve the same model set — and it picks the instance with a mutex-guarded round-robin counter, so consecutive commands walk the instances in turn. Inside a pool, a second counter rotates across that pool's connections, one command per withdraw; connections are created up front but connect lazily on first use. The reason for rotating connections rather than reusing one is that behind a connection-level load balancer such as AWS Service Connect, an NLB or an Envoy TCP proxy, each keep-alive connection is pinned to a single upstream instance, so spreading commands across the pool spreads load across every instance even from a single thread at zero concurrency. Only one class of failure is retried: a pre-send connection error, where the connection was never established and therefore nothing was written. The router then moves to the next instance, for at most as many attempts as there are pools, and raises the last error if they all fail. Errors that can arrive after a command may have been sent — a read timeout, or a connection lost mid-flight — are never re-dispatched, because the server may already have run the inference. The router also survives fork: a hook on Process._fork resets every registered pool in the child, closing inherited sockets and rebuilding the mutexes and counters so parent and child never share a connection, which is what makes Puma preload, unicorn and resque safe. Pools are tracked in a WeakMap so they are reclaimed with their client, and a nested with from the same thread re-enters the connection that thread already holds instead of taking a second one.">

The retry rule is the sharpest edge in the client: **only a failure that
happened before the command was written is retried.** A connection that was
never established can be moved to another instance safely. A read timeout
cannot, because the server may already have run the inference. `EMB.MULTI` is
not idempotent, so a re-send is duplicated work, not a correction. This is why
`reconnect_attempts` defaults to `0` and why the loopback health check
(`EMB.READY`) exists: it is cheaper to fail closed than to guess.

## Choosing a mode

| Mode | Round trips | Latency shape | Use when |
|---|---|---|---|
| `false` (default) | one per call | predictable, one text in flight | interactive paths, one or two texts |
| `:multi` | one per 512 texts, serial | coalesced, still ordered | batches inside one request, simple to reason about |
| `:batch` | one per 512 texts, parallel | highest throughput | bulk backfills, offline jobs, many texts |

All three keep the same reply shape, so switching is a one-word change.
