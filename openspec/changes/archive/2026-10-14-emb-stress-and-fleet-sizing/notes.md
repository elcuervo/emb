# Verification notes

Recorded observations from running the change against a real, local fleet.
These are evidence, not a spec: nothing here is asserted by CI.

## 7.1 — a two-node fleet, driven by `emb-stress`, watched by `emb-top`

**Setup.** One DNS name, two nodes behind it. `localhost` resolves to
`127.0.0.1` and `::1`, so a bare `-target localhost` / `-node localhost`
expands to two endpoints at the default port 6379:

| Node | Listen | `cpu_budget` | Model |
|---|---|---|---|
| A | `127.0.0.1:6379` | `2` (deliberately constrained) | minilm, `max_length: 256` |
| B | `[::1]:6379` | default (detected 10 cores → budget 8) | minilm, `max_length: 256` |

```bash
emb-top  -node localhost -frames -interval 1s > /tmp/top-frames.jsonl &
emb-stress -config a.yaml -target localhost -levels 8 -dwell 18s -warmup 4s -seed 3
```

### The two tools agree, and the client agrees with the server

`emb-stress` offers 8 workers to each endpoint — a 50/50 **offered** share.
The nodes do not serve it evenly, because node A's CPU budget binds first:

```
FLEET
  level 1 · workers 16 · offered 225.9/s · completed 225.9/s · p50 66.0ms · p95 132.6ms · p99 138.7ms · 0 errors

[::1]:6379
  level 1 · workers 8 · offered 148.7/s · completed 148.7/s · p50 55.7ms · p95 69.4ms · p99 76.5ms · 0 errors

127.0.0.1:6379
  level 1 · workers 8 · offered 77.1/s · completed 77.1/s · p50 107.9ms · p95 135.6ms · p99 149.5ms · 0 errors
```

The same run, counted from the servers' own `total_requests` deltas (including
each level's discarded warm-up):

| Node | Offered share | Completed share (`emb-stress`) | `total_requests` delta | Share |
|---|---|---|---|---|
| A `127.0.0.1:6379` | 50% | 77.1/s | 1268 | 34.5% |
| B `[::1]:6379` | 50% | 148.7/s | 2403 | 65.5% |

So the *offered* share is even, the *observed* share is not, and the client's
own count reconciles with the server's counter exactly (77.1/s × 10s measured
window + 4s of warm-up ≈ 1268 on A, 148.7/s × 10s + 4s ≈ 2403 on B). That gap
is the thing the pairing exists to show: a node offered its 1/N that serves
less is the node to look at.

### The fleet recommendation names the saturated node

Every distinct fleet sizing line `emb-top` rendered during the run:

```
SIZING  add nodes: every node's cpu headroom is small (fleet headroom 0%)
SIZING  inspect 127.0.0.1:6379 first · add nodes: every node's cpu headroom is small (fleet headroom 0%)
SIZING  inspect 127.0.0.1:6379 first
```

`127.0.0.1:6379` — the node the run saturated, and the node whose client-measured
p95 is far above its peer's (135.6ms vs 69.4ms) at comparable offered load — is
the node named first. The capacity clause appears only while the fleet's CPU
headroom really is spent (0%), and is absent once it is not.

### What this run found in the tools

Three defects surfaced only under a real fleet, all fixed in this change:

1. **Workers shared one socket.** A RESP connection carries one request and one
   reply at a time; a level's workers interleaving their writes and reads on one
   connection corrupted the stream and reported it as load (a level of 60k
   spurious errors, then a level with no samples at all). Each worker now opens
   its own connection, and the level curve is what a closed loop should look
   like: 143/s at 1 worker, 148/s at 2, 172/s at 4, 185/s at 8, with p50 rising
   7.1ms → 14.1ms → 23.6ms → 45.7ms.
2. **A not-ready model was exercised.** A fleet that has just started answers
   `ERR not ready: model "x" is initializing` until its layout is decided, so a
   run against a cold fleet reported the server's own startup as tens of
   thousands of load errors. Reconciliation now treats a model whose
   `EMB.MODELS` status is not `ready` as not exercised, names it, and the run
   exits non-zero when nothing was exercised.
3. **A multi-address name labelled every row identically.** A name that expands
   to several addresses produced rows all named by the shared name, so the
   cluster view showed two rows called `localhost`. Expanded rows are now named
   by the address they dial, which is what makes the offered/observed comparison
   above legible at all.
4. **The growth rule fired on allocator noise.** Run against a live fleet, the
   sizing line reported `rec=growth` on every poll of a steady run: RSS creeps a
   little under load even when nothing leaks, and the rule as first written only
   required a monotone rise. It now needs the rise to clear a magnitude floor
   (10% of the window's starting RSS, and at least 8 MB), and the same run
   reports `rec=nodes` — the fleet is out of CPU headroom, nothing is leaking —
   which is the honest answer.

### The headless form on the same run

`emb-top -node localhost -once -samples 20 -interval 1s`, run alongside:

```
rec=nodes
```

— the fleet's own advisory, as one token on the aggregate line, with each
`node:<label>` section carrying its own. Absent when nothing is near a
threshold.
5. **A worker never re-dialled.** Each worker opens its own connection at the
   start of a level, and `Embed` on a dropped socket failed with "resp: not
   connected" for the rest of that level: one lost connection turned into every
   remaining request of the level being counted as an error. The worker now
   re-dials before each request, so one lost socket costs one error. Found by
   the endpoint test written for this review, not by a live run.
