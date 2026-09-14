## Context

See `proposal.md` — Why. The constraints that shape the approach:

- `emb`'s command surface computes embeddings and nothing else. There is no
  index, no similarity function, and no ranking command; cosine exists only
  inside `internal/embverify` as Go test tooling. Any end-to-end example
  therefore *must* bring a second store.
- Redis 8.8 is present in the development shell and ships the `vectorset`
  module. Verified live: `VADD`, `VSETATTR`, `VSIM … WITHSCORES WITHATTRIBS`,
  `VEMB`, `VDIM`, and `VCARD` are all available, and `VSIM`'s score is
  magnitude-invariant — but it is **not** a raw cosine. Redis rescales it to
  `(1 + cos) / 2`, verified against known vectors: orthogonal scores 0.5 and
  opposite scores 0.0.
- `gems/emb` already depends on `redis-client ~> 0.24`, and `Emb::Client`'s
  constructor takes a URL and exposes a generic `#send_command`. So a client for
  each server costs no new dependency.
- There is no `emb.yaml` convention anywhere: the server accepts `-config
  <path>`, with no auto-discovery and no environment variable. The name is a
  filename choice for this example, not a new mechanism.
- `examples/` currently holds only `examples/scripts/*.lua` plus their tests —
  constructs, one model at a time, with no application around them.
- The product site's traceability rule names `README.md`, `BENCHMARK.md`,
  `examples/scripts/`, and the shipped code as permitted sources, and its
  console panel is deliberately withheld from the rendered page until a live
  executor exists.

Two measured properties of the index shape the design more than the prose does:

| Probe | Result |
|---|---|
| `VADD f VALUES 3 0.123456789 …` then `VEMB f elem:float` | `0.12442833930253983` — vector sets quantize (Q8) by default; stored vectors are approximate |
| `VSIM … WITHSCORES WITHATTRIBS` | flat `[element, score, attributes]` per hit; `[element, attributes]` without `WITHSCORES`; `attributes` is `nil` for elements that carry none |

## Goals / Non-Goals

**Goals:**

- Show the product's contract — compute here, store and search elsewhere — in
  runnable form, in about a screen of code.
- Make every claim the example's documentation makes reproducible by a command.
- Cost nothing to adopt: no new dependency, no server or client change, no CI
  surface.

**Non-Goals:**

- Search quality, recall measurement, or benchmarking. The example demonstrates
  a composition, not a tuned index.
- A reusable client library or a vector-store abstraction. There is exactly one
  implementation of each role here; an abstraction over one implementation is
  cost without a second caller.
- Website changes. See Open Questions — the constructions worth extracting are
  named below so that work can be written against a settled example.
- The heavier tiers in the lean path. Cross-modal search, reranking, and
  extraction are designed as commented configuration, not as working code.

## Decisions

### D1 — One client per server, each the natural one for its role

`Emb::Client` talks to the `emb` instance; `RedisClient` talks to Redis. Both
come from gems the repository already ships.

*Alternatives.* Routing Redis commands through `Emb::Client#send_command` would
work — it is the same protocol and the same underlying gem — and would reduce
the example to a single client object. Rejected: it reads as a misuse, makes the
two roles look like one thing, and teaches a reader that `emb` and Redis are
interchangeable. A thin wrapper exposing `embed` and `search` over both was also
considered and rejected for the same reason the specs' Non-Goals give: it
abstracts over a single implementation.

### D2 — The payload lives in the index, not beside it

Each chunk is stored as one element: `VADD` carries the vector, `VSETATTR`
carries `{"src", "body"}`, and `VSIM … WITHSCORES WITHATTRIBS` returns the
element id, the similarity score, and the text in one reply.

*Alternatives.* A parallel Redis hash keyed by element id is the obvious design
and is what most hand-rolled vector demos do. Rejected because it creates a
second source of truth that can drift from the index — the failure mode with
`DEL`, a partial write, or a failed pipeline — for no benefit. Storing the text
in the element name was rejected as an encoding abuse.

Because the payload is in the index, the whole corpus is one Redis key, and the
example has no consistency story to tell.

### D3 — The lean path is one model, text to text, over documents

The lean path is `minilm` (~86 MB), a document corpus, and pure vector search.
Reranking, extractive QA, GLiNER extraction, and cross-modal image search are
each one extra model download of 300 MB or more; they appear as a commented
configuration block and are documented only when implemented.

*Alternatives.* A cross-modal `siglip2` asset library is the most striking demo
of `EMB.IMG` and was the strongest runner-up. Rejected for the lean path because
it needs a large vision export *and* image assets to index, which turns "run the
example" into an acquisition step. The full sink — every capability, nothing
excluded — was rejected as the default because a demo that takes ten minutes to
start is not a demo.

The tiering exists so the example can grow without the lean path changing.

### D4 — Content-addressed chunk ids, full rebuild on index

A chunk's id is a digest of its source path and its text. `index` begins with
`DEL`.

*Alternatives.* Sequential or positional ids make an edit shift every
subsequent id and force a full rewrite with no benefit. Omitting the `DEL` in
favour of pure upsert was considered: content-addressed ids make it correct for
additions and edits for free. Rejected as the default because it is silently
wrong for *deletions* — a paragraph removed from a source file would linger in
the index forever, with no way to notice. `DEL` makes the rebuild honest; the
content-addressed ids remain valuable because they make the rebuild's writes
idempotent and keep an edit's blast radius to the chunks it touched.

### D5 — Ingestion batches, and a failed chunk degrades only itself

Chunks are embedded through one `EMB.MULTI` per slice of 512 — chosen to sit
comfortably under the server's default `max_pairs` of 4096 — and stored through
a Redis pipeline.

`EMB.MULTI` answers MGET-style: a failed pair yields `null` in its own slot
rather than failing the command. The example skips those chunks, continues, and
reports the count. A null vector is never stored, because a zero vector in an
index is worse than an absent element: it matches nothing, but it looks indexed.

### D6 — Configuration carries only fields whose effect is observable

`emb.yaml` sets `preload: true`, `normalize: true`, `cache: auto`, and a single
`model_repo`. Each is chosen because a reader can *see* it from outside:
preloading moves the model-load stall to startup, `EMB.STATS` prints
`norm=true`, and caching makes a repeated search cost one inference instead of
two. Fields whose effect cannot be observed in the example are omitted rather
than left at defaults, so the file reads as a set of reasons.

Normalization is pinned rather than defaulted because the repository's default
is `false`. `VSIM` does not need it — a cosine is scale-invariant, so the index
ranks identically either way — but normalizing means the vectors handed to the
client are directly comparable by plain dot product, so app-side math and any
future dot-product index agree with what the server reports. Pinning it keeps
that from being a silent choice.

### D7 — The corpus is the repository's own documentation

`index` is pointed at `README.md`, `DESIGN.md`, and `website/PRODUCT.md`.

*Alternatives.* A committed corpus file is hermetic and reproducible across
checkouts, but goes stale and must be maintained. A downloaded corpus is
neither reproducible offline nor licence-free. The repository's own docs are
zero-weight, always in sync, and make the demo answer real questions about the
project — which is the only kind of demo that survives being run by a skeptic.

### D8 — The score's real meaning is documented, not smoothed over

`VSIM`'s score is `(1 + cos) / 2`, not a raw cosine, so a score of 0.5 means
*orthogonal* and 1.0 means identical. Ordering is unaffected — the rescale is
monotonic — but the number is easy to read as a similarity percentage and be
wrong by an amount that grows as results get worse.

The example prints the server's number unchanged and explains the mapping in
the README, rather than converting it to a true cosine in `app.rb`. Converting
would be friendlier and would also hide the single most misleading number the
server returns; a reader who never learns the mapping will meet it again in
their own code. The README states the measured relationship — a reported 0.693
is a cosine of 0.386 — so the claim is checkable rather than asserted.

Redis vector sets quantize by default, so stored vectors and scores are
approximate — `0.123456789` reads back as `0.12442833930253983`. Measured
against brute-force cosine over this corpus, the quantization costs about
`0.0005` of score: enough to reorder two results that close together, and
nothing more. This is a documented trade with a one-token escape hatch
(`NOQUANT`), and the README states it instead of letting a reader discover it
and distrust the numbers.

### D9 — The servers stay up between invocations

`run.sh` starts Redis and `emb` only if they are not already running, records
pidfiles under a gitignored `examples/kitchensink/.state/`, and leaves them up;
`run.sh stop` shuts them down. Redis persists append-only in that directory.

The runner hands `emb` the ONNX Runtime library explicitly (`-ort-lib <file>`)
rather than exporting `DYLD_LIBRARY_PATH`, because macOS drops `DYLD_*` when a
script is executed through `/usr/bin/env` — which is exactly how a script with a
`#!/usr/bin/env bash` shebang starts. The dev shell's path therefore never
reaches the script, and the server fails with a `dlopen` error naming
`bin/libonnxruntime.1.dylib`. Pointing the server at the library sidesteps the
loader entirely and works whether or not the caller was already in the shell.
The library is found by globbing `/nix/store` (about 40 ms) unless `ORT_LIB` is
set. Do not replace this with an inherited environment variable; it will appear
to work when the script is invoked as `bash run.sh` and fail when it is invoked
as `./run.sh`.

*Alternatives.* Starting both servers per invocation and tearing them down on
exit is tidier in isolation, and it was the first implementation. Rejected
because it makes two of the example's own configuration choices meaningless: a
freshly started `emb` has an empty cache, so `cache: auto` never pays off and
the cache cannot be demonstrated across the `index` and `search` invocations
the README asks the reader to run. A persistent pair is also what anyone
doing this for real would have. Redis persists append-only rather than by
snapshot so the index survives a crash and not only a clean stop.

### D10 — Batching is evidenced by the example's output, not by server counters

`index` reports one line per slice (`batch 1: 184/184`), so the number of
embedding commands is visible in the example's own output and changes if the
batching regresses.

This is a deliberate retreat from the obvious verification, which does not
work. `EMB.STATS`'s `total_requests` counts **commands** for `EMB` (one
command carrying three texts increments it once) but counts **pairs** for
`EMB.MULTI` (three pairs increment it three times), and `MONITOR` emits one
event per pair. So neither counter can distinguish "one `EMB.MULTI` carrying
184 chunks" from "184 separate commands" — the very thing batching changes.
The inconsistency between the two commands' accounting is noted below; it is
not this change's to fix.

## Risks / Trade-offs

- **[The example needs a Redis with the `vectorset` module]** → The development
  shell provides 8.8, and the requirement is stated in the example's README. The
  failure is loud and immediate (a command error on the first `VADD`), not
  silent. No fallback path is provided, because a fallback would mean
  reimplementing search in application code — exactly what the example exists to
  argue against.
- **[Quantization makes results approximate]** → Documented with the measured
  number, and the `NOQUANT` escape hatch is named. The example does not present
  its scores as exact.
- **[The corpus drifts as the docs change]** → Content-addressed ids make
  re-indexing cheap and idempotent, and `index` rebuilds from scratch, so drift
  is a re-run rather than a corruption.
- **[`total_requests` counts inconsistently across commands]** `EMB` with N texts
  increments it once; `EMB.MULTI` with N pairs increments it N times, so the
  counter is not comparable across the two paths and cannot evidence
  client-side batching. → Not fixed here (this change touches no server code);
  the example depends on it nowhere and design D10 avoids it. It is a candidate
  follow-up, since a reader comparing the two commands' throughput will hit it.
- **[The example can rot — nothing in CI runs it]** → It adds no dependency to
  rot, and its only external surfaces are RESP commands the server already
  tests. The specification's reproducibility rule keeps the documentation
  honest; making the example a CI gate is deliberately out of scope, because it
  would add a model download and two servers to the test path.
- **[Port collision]** `just all` uses 16379 for its own server. → The example
  documents the collision and its own port choice.
- **[First run downloads ~86 MB]** → Named in the README, and `preload: true`
  makes the load appear at startup rather than as a slow first query.

## Migration Plan

Additive and self-contained. Nothing to migrate: a new directory plus two
additive edits (a task-runner target and a README pointer). Rollback is deleting
`examples/kitchensink/` and reverting those two edits; no state outside the
example's own Redis instance is touched.

## Open Questions

- **The website extraction is a follow-up change.** The site's traceability rule
  lists permitted sources, and its console panel is withheld until a live
  executor exists. Two constructions from this example are worth extracting when
  that change is written: the *two-server composition* (an `emb` call and an
  index call in one transcript, showing that neither server pretends to be the
  other), and the *one-element index* (`VADD` + `VSETATTR` → `VSIM` +
  `WITHATTRIBS`, showing the text coming back out of the index). Both are
  transcripts the site's rules permit once the example is the source. Deferred
  rather than folded in because the site has its own change in flight and its
  own design and publishing rules.
- **Whether `emb.yaml` should ever be discovered rather than passed.** The name
  is currently just a filename in this example. Making the server discover it
  would introduce a precedence question against `-config` and the repository's
  existing configuration files, for the benefit of one example. Not needed for
  this change; noted because the name will make someone ask.
