# Verification notes

Recorded observations from running the change against a real, local node. These
are evidence, not a spec: nothing here is asserted by CI.

## The node

`bin/emb` built from this change, two models preloaded, a 64 MB reply cache:

```bash
./bin/emb -config test-two-models.yaml -listen 127.0.0.1:16449 -cache 64MB
```

Both models laid out as batchers (`batching=1ms/32/16384`, `workers=1`,
`intra_op_threads=4`), process budget 8 of 10 cores. Load was driven with two
shapes at once — repeated short texts (cache hits) and fresh ones (runs) —
through `redis-cli` loops while `emb-top` sampled.

## The advice is an action, and it names what it rejected

`./bin/emb-top -addr 127.0.0.1:16449 -once -samples 5 -interval 1s` under
sustained load, last line:

```
act=model act_rejected=cpu_budget cached_pct=48.3 lat_p95_run_us=3018 lat_p95_run_n=885
phase=run phase_share=99.8
```

The node's uncached tail was 99.8% inference, so the answer is the model, not the
budget — and the line says so in as many words. Half the traffic never reached a
session (`cached_pct=48.3`), which is why the CPU readout must be read per run
rather than per request.

## The per-model phases reconcile with the server's own average

Same run, the `bge` section:

```
model:bge dim=384 reqs=876 toks=6132 errs=0 avg_latency_us=2690 … intra_op_threads=4
  cache_served_reqs=0 runs=256 wait_us=6 admit_us=0 run_us=2679
```

256 runs of 2.679 ms mean ORT time, 6 µs mean wait for the batch window, no gate
wait — against the server's own cumulative `avg_latency_us=2690` for the same
window. The three phases are the request's own, and the numbers agree with an
independent counter.

## The rendered panel

`-frames` under load renders two lines: the action with the counters that decided
it, then the rejected action and the window's own facts (`served 50% from cache ·
200.0ms cpu/run`), with the latency band's legend separating the service tail
from the runs' (`runs p95 … · n=…`). The decision this replaced — an idle node
with 10 ms of stolen time and no work reporting `host is the constraint` — is now
quiet, and is asserted in `internal/embtop/advice_test.go` and
`recommend_boundary_test.go`.

## The documented capture carries the new readout

`just website-topviz` re-recorded the plate, the still frame and the animated GIF
from a real four-model node under the scripted load, and published them
(`website/assets/cast/emb-top-e348a08c.cast`, `docs/assets/emb-top-521dc32b.gif`).
The frame that goes into `website/docs/index.html` and the plate opens with:

```
SIZING  take work off the model — runs hold 96% of the uncached tail (p95 36.5ms)
not more cpu: the time is inference, not queueing · served 96% from cache · 18.6ms cpu/run
… p50–p99 ░  p95 █  runs p95 36.5ms · n=1824
```

Every figure is the recorded run's own: the action, the phase share behind it,
the cache's share of the traffic, the CPU per uncached run and the runs' p95 with
the count it rests on. The published-tree check passes with the new names, and the
previous artifacts were removed by the capture rather than left behind.

## Release notes for the next version

Two wire changes, both additive, plus one reporting change:

- **`MONITOR` events carry the request's attribution.** Four fields are appended
  after `err` under both encodings — `cached_texts`, `attributed`,
  `dispatch_us`, `admit_us`, `run_us` (RESP2: elements 7-11 of a flat row; RESP3:
  the same four keys). A client reading the original six positionally is
  unaffected; one that pins the element count has to be updated. `attributed` is
  0 for a request whose phases the server did not measure (an image run), which
  means "not measured", never "a free run".
- **`EMB.INFO <model>`'s `cache_*` counters are that model's own**, not the
  process's. Field names and RESP array counts are unchanged; a consumer
  comparing two models' `cache_hits` now sees two different (correct) numbers,
  and `EMB.STATS` still carries the aggregate.
- **`emb-top`'s sizing panel is now an action**, with a second line naming the
  action it rejects and the window's facts, and every `-once`/`-frames` token it
  already carried keeps its name.
