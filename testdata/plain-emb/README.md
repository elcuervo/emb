# Plain-EMB gate fixture

The committed fixture the plain-`EMB` gate drives: `internal/server/emb_smoke_test.go`,
reached by `just bench-plain-emb` or

```sh
EMB_BENCH_SMOKE=1 go test ./internal/server/ -run PlainEMB -v -count=1 -timeout 600s
```

The test is named `TestPlainEMBSmoke`, so the sweep's `-run Smoke` reaches it as
well — `bench/smoke/verify-gate.sh` runs both gates in one `go test`, which is
what lets a tier-2 mutant be caught by whichever gate observes it.

## Why a second miniature

`testdata/laya/model.onnx` cannot serve the plain embedding path, which is the
path the incident was about. Its graph declares five inputs — `input_ids`,
`attention_mask`, `marker_pos`, `marker_mask`, `qtype` — while the embedding pool
feeds only the first two, so every plain request fails:

```
  plain: inputs=[input_ids attention_mask marker_pos marker_mask qtype] output="last_hidden_state" rank=3
  plain: batch determinism probe failed (probe solo run A: inference: onnx run:
    The session specified 5 input names, but Run() was called with 2 input tensors)
$ EMB plain hello world
ERR inference: onnx run: The session specified 5 input names, but Run() was called with 2 input tensors
```

Narrowing the session's input list does not help: ONNX Runtime itself refuses the
run (`Missing Input: qtype`), because the export derives its batch dimension from
`qtype`'s shape. The laya tokenizer is reused as-is
(`testdata/laya/tokenizer/tokenizer.json`, WordLevel, 92 entries, ids 0..91), so
the corpus's tokens are real ids rather than all `[UNK]`.

## What is here

- `build-model.py` — the miniature's generator, Python standard library only.
  Deterministic by construction: the weights come from a fixed LCG seed, so a
  re-run writes byte-identical output. **Nothing in CI runs it**; the committed
  bytes are authoritative, and the gate fails when `model.onnx` no longer matches
  the digest `expected.json` records.
- `model.onnx` — the generated plain-embeddable miniature (595,612 bytes).
- `texts.json` — the fixed corpus: eight texts, from two words to ~1,100 words,
  built from the laya vocabulary. No ARGV, no randomness, no client-side
  generation.
- `expected.json` — the pins: `dim` plus, per text, the SHA-256 of the
  little-endian float32 bytes the server replied with, and the decoded vector (so
  a mismatch is diagnosable). It also records the model digest, the tokenizer and
  the config the pins were taken with; the gate refuses to run when any of them
  has drifted, and never silently skips a missing or corrupt fixture.

## The miniature

```
input_ids [batch, seq] i64,  attention_mask [batch, seq] i64
  Gather(tok_table, input_ids)                  -> [batch, seq, 128]
  Tanh(h @ W_i + b_i)                    x 8    -> [batch, seq, 128]
  Mul(h, Cast(Unsqueeze(attention_mask, -1)))   -> last_hidden_state
```

It is deliberately not a transformer: what the gate needs from a model is a
deterministic output and a per-run cost that scales with the token count, and the
dense stack gives both in a graph small enough to commit. dim 128, vocab 128, 8
layers; 2.5 ms for one 512-token row and ~7 ms for a 1,100-token row on the
reference machine (darwin/arm64), which is what lets a burst fill the admission
gate instead of finishing inside one scheduler tick. The gate drives it with
`pooling: mean`, `normalize: true`, `max_length: 2048`, batching on for the
`plain` arm and off where an arm needs the worker pool.

## Regenerating

```sh
# the model (rarely: only when the graph or the shape changes)
python3 testdata/plain-emb/build-model.py

# the pins (after any change to the model, the tokenizer, or the fixture config)
EMB_BENCH_SMOKE=1 EMB_PLAIN_EMB_UPDATE=1 \
  go test ./internal/server/ -run PlainEMB -v -count=1 -timeout 600s
```

Update mode runs the serial phase and writes `expected.json`; it asserts nothing
and prints `emb-smoke: regenerated testdata/plain-emb/expected.json (8 pins)`.

The digest is the pin. A host whose ONNX Runtime kernel keeps different low bits —
another architecture, another ORT build — can differ in the last ones, which is
why the gate also pins the vector and accepts a digest mismatch only when every
component is within `1e-5`; anything larger is a real change. That looser path is
never silent: the outcome ends with `digest-mismatch-tolerance on text(s) [i]`
(and `all digests exact` when no text took it), and the run also logs a
`digest-mismatch-tolerance` line per affected text. The recorded values are from
darwin/arm64.

## What the gate asserts, and what it measured

Twelve named assertions, every one of them exercised on the reference machine
(darwin/arm64, 10 CPUs, budget `cpu_budget: 2`). Runs on the clean tree are green; the same gate run against a tree with the
planted FIFO-head regression (`internal/permit/budget.go`: a `return -1` before
the borrow loop, so the head blocks every follower) goes red **naming
`admission-fair`**, with the probe measured at 2.2 ms against the same bound.
Five consecutive runs:

| assertion | what it observes | measured (5 runs) | bound |
| --- | --- | --- | --- |
| `plain-emb-pinned` | serial phase: dim and reply digest per corpus text | 8/8 texts exact | digest; a digest mismatch is accepted only within 1e-5 per component, and such a run says `digest-mismatch-tolerance on text(s) [i]` in the outcome and the log |
| `cache-idle` | a second pass over the corpus is a cache hit and does not advance the run counter | 8 hits, 0 runs | 0 runs |
| `plain-emb-latency` | batch-on burst of 48 commands × 4 texts: p99 | 9.5–14.5 ms | 250 ms |
| `health-answers` | `PING` + `EMB.READY` answered on every probe across serial, burst and drain | 67 answered, 0 unanswered | 0 unanswered |
| `drain-after-disconnect` | gauges fall back, counters freeze, a client that walks away mid-command leaves nothing running, the pins still hold | drained, frozen (53→53 requests, 16→16 runs), wound down | — |
| `calibration-off-request-path` | the layout is decided at load (`calibration_source` reported), no layout line after load, first request bounded; and on the `lazy` arm (no `preload`) the first request is answered `not ready` inside the bound while the load runs in the background | 1 line at load, 0 after, 0.65–0.88 ms first request (preloaded); `lazy` arm not-ready at boot, first request a bounded refusal, served after the load | 100 ms |
| `queue-bounded` | 32 concurrent long-text commands against 16 sessions at a budget of 2: counted refusals and observed queue depth | 25–26 refusals, depth peaked at 4 | depth ≤ 4, refusals > 0 |
| `admission-bounded` | waits that cannot be granted inside the derived bound end as counted refusals; the rail on how long a wait may run is the queue shape (queue bound + runs in flight + its own run) at twice the measured serial cost, so a shared runner's load does not read as a collapse | 5–11 expired waits, max 5.9–7.2 ms | bound = serial/3, limit = bound + 14×serial |
| `admission-wait-reported` | the serial phase reports no wait; the burst reports a non-zero one | 0 µs, then 9.9–11.6 ms | — |
| `layout-concurrency` | the policy layout gives two concurrently admissible runs at a budget of 2, and more than one run is in flight under a burst | workers 2 × 1 thread, peak ≥ 2 active tokens | workers ≥ 2, `workers×threads ≤ budget`, peak ≥ 2 |
| `admission-fair` | two models on one budget of 3 — `wide` at the whole budget, the fixture at one thread — with the wide run parked behind a holder: a narrow probe is served into the idle capacity | probe 193–195 µs against 165–178 µs serial while the head had waited 2.5 ms (holder 3.3 ms) | 5 × the probe's own serial latency |

Cost: the gate's test body runs in 0.6–0.8 s (whole `go test` invocation
1.06–1.5 s including the test binary). It adds that to whatever job runs it —
`just bench-plain-emb`, the CGo job, and `-run Smoke`.
