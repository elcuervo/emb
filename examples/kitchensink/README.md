# Kitchensink

The smallest end-to-end vector application on top of `emb`.

`emb` turns text into vectors. It does not store them and it cannot search them —
and that is the point of this directory. This is the other half:

```
   emb   :16400    text    →  vector
   redis :6399     vector  →  ranked hits, and the text back
```

Two servers, one protocol (RESP), one client library each. `app.rb` is about 90
lines of code and nothing in it is clever; the servers absorb every hard part, so
all it does is ingest, query, and report.

## Run it

Inside `nix develop`:

```bash
examples/kitchensink/run.sh index README.md DESIGN.md website/PRODUCT.md
examples/kitchensink/run.sh search "how does batching work" 3
examples/kitchensink/run.sh stats
```

`run.sh` starts both servers if they are not already running and leaves them up,
so the second command is fast and the cache stays warm. `run.sh stop` shuts them
down. The index and the pidfiles live in `examples/kitchensink/.state/` — delete
that directory to start over.

The app runs under the `gems/emb` bundle — the same one `just all` uses — so run
`cd gems/emb && bundle install` once if you have not already.

Indexing the repository's own documentation keeps the corpus free and in sync
with the code it describes.

## What it actually prints

```
$ examples/kitchensink/run.sh index README.md DESIGN.md website/PRODUCT.md
embedding 184 chunks from 3 files
  batch 1: 184/184
indexed 184 elements at dim 384

$ examples/kitchensink/run.sh search "how does batching work" 3
   1.  0.693  README.md        Batching is **on by default** for every model, so no config is n
   2.  0.689  README.md        The server decodes the image (PNG/JPEG/GIF/WebP), resizes/crops,
   3.  0.683  README.md        Automatic and manual saves briefly capture immutable entry descr

$ examples/kitchensink/run.sh stats
  emb    requests=185 tokens=13825 errors=0
  model  minilm: req=185 avg=6650us tok=13825 err=0 pool=mean norm=true batch=1/32 budget=16384 eff=0.670
  cache  hits=0 misses=185 rate=0.0%
  index  elements=184 dim=384
```

The latencies vary with machine and load; the counts do not.

Search the same query again and only two numbers move:

```
  cache  hits=1 misses=185 rate=0.5%      # hits went up
  emb    requests=185 tokens=13825        # requests did not
```

A repeated query is answered from `emb`'s cache, so the second search costs no
inference at all. The same mechanism makes re-indexing an unchanged corpus free:
run `index` again and `hits` climbs to 185 while `requests` stays at 185.

## How this was checked

The transcript above is not the only evidence, and "the results looked relevant"
is not evidence. Retrieval was verified against an independent reference: the
corpus re-derived in Ruby, every stored vector pulled back with `VEMB`, and the
cosine recomputed by hand for comparison.

Run against the client **in this working tree** — `gems/emb`, loaded by the
bundle as a path gem, so the example exercises the repository's own code and not
an installed release:

```
client   gems/emb/lib/emb.rb  v0.4.0.pre4
index    elements=184 dim=384
  PASS  round trip: the top hit is the chunk whose text was the query
  PASS  the returned text belongs to the vector that matched
  PASS  identical text scores 1.0
  PASS  score == (1 + cos)/2 across 30 pairs (max deviation 0.00053)
  PASS  VSIM order == brute-force cosine order (3 queries x top 5)
  PASS  an unrelated query tops out at 0.6226 -- cosine 0.245 -- below the self-match
ALL CHECKS PASSED
```

The failures each check rules out:

| Check | What it would catch |
|---|---|
| round trip | a query returning some other chunk, or querying the wrong key |
| text belongs to the vector | `VADD` and `VSETATTR` writing to mismatched elements — the index would return right-looking vectors under wrong text |
| `(1 + cos)/2` | misreading the score, which is the mistake this README makes easiest (see Gotchas) |
| brute-force order | `VSIM` not actually returning nearest neighbours |
| unrelated query ranks lower | an index returning arbitrary elements with plausible-looking scores |

These were run by hand. Nothing in the repository's test path starts two servers
and a model, so none of this is gated in CI — the numbers are reproducible from
the commands above, not guarded.

## What each half does

| Step | Command | Why it matters |
|---|---|---|
| embed a slice | `EMB.MULTI` | one round trip for up to 512 chunks, not 512 round trips |
| store a chunk | `VADD` + `VSETATTR` | the vector and the text land in **one element** |
| search | `VSIM … WITHSCORES WITHATTRIBS` | returns element, similarity score, and the text |
| prove it | `EMB.STATS`, `INFO cache` | numbers, not claims |

Because the payload rides in the index's own attributes, there is no second
store to keep in sync. The whole corpus is one Redis key — `KEYS *` shows
exactly `kitchensink:docs`, a `vectorset` of 184 elements at dim 384.

Searching also takes a filter, which is the version people need in production:

```bash
$ examples/kitchensink/run.sh search "how does the cache behave" 3
   1.  0.744  README.md        Sandbox notes: scripts are **pure compute** — `os`, `io`, `requi
   2.  0.725  README.md        Startup restore streams into an unpublished staging cache. Its e
   3.  0.713  README.md        | Setting / CLI flag | Default | Live update | Meaning | |---|--

$ examples/kitchensink/run.sh search "how does the cache behave" 3 --filter '.src == "DESIGN.md"'
   1.  0.627  DESIGN.md        Each element moves 10px and fades over 280/420ms on an exponenti
   2.  0.624  DESIGN.md        1. header fades or snaps into position 2. tiny annotations appea
   3.  0.589  DESIGN.md        `BLOB OR VALUES`, `HELLO 3` and `1 MS WINDOW` were the three bra
```

## emb.yaml

`listen` and one model are the only required fields. Everything else is here
because its effect is visible from outside the server:

| Field | Why |
|---|---|
| `preload: true` | the first query must not pay the model load |
| `normalize: true` | makes the vectors the client receives comparable by plain dot product, so app-side math agrees with the index. `VSIM` does not need it — its score is scale-invariant — but the repository default is `false`, so it is pinned rather than assumed |
| `cache: auto` | a repeated search costs one inference, not two — and a re-index of unchanged text costs none |
| `max_length: 256` | chunks are short; longer sequences buy nothing here |

`normalize: true` is visible in the `stats` output above (`pool=mean norm=true`),
so an accidental edit to this file is not invisible.

`scripts:` paths resolve relative to `emb.yaml` itself, which is how the
commented Tier 2 block reaches `../scripts/rerank.lua`.

## Tier 2: reranking

Vector search is recall-oriented; a cross-encoder is precision-oriented. The
`reranker` model is present in `emb.yaml` as a **commented block**, and the
README stops there: `app.rb` does not implement a rerank path yet, so nothing
here tells you to run one.

## Gotchas

- **The score is not a cosine.** Redis vector sets rescale it to `(1 + cos) / 2`,
  so the numbers above run from 1.0 (identical) through **0.5 (orthogonal)** to
  0.0 (opposite). The top hit at `0.693` is a true cosine of `0.386`, not 0.693.
  Ranking is unaffected — the rescale is monotonic — but reading the number as a
  similarity percentage is off by a factor that grows as results get worse.
  Measured against brute-force cosine over the same vectors, the reported score
  matches to within `0.0005` (the Q8 quantization error).
- **Query and index must agree.** Same model, the same normalization, the same
  chunking. Flip `normalize` and ranking degrades *silently* — which is why it is
  pinned in the config rather than left at the default.
- **Redis vector sets quantize by default.** `VADD f VALUES 3 0.123456789 …`
  reads back through `VEMB` as `0.12442833930253983`, and a normalized vector
  measures `‖v‖ ≈ 1.0004` rather than exactly 1. In this corpus that is worth
  about `0.0005` of cosine — enough to swap two results whose scores differ by
  less than that, and nothing more. `VADD … NOQUANT` trades memory for
  exactness.
- **`index` begins with `DEL`.** Chunk ids are a digest of source and text, so
  re-indexing is idempotent and re-runs are cheap. The `DEL` is still there
  because without it a paragraph *deleted* from a source file would linger in the
  index forever.
- **Requires a Redis with the `vectorset` module** (Redis 8). The Nix dev shell
  provides one. There is deliberately no fallback that searches in Ruby — that
  would reimplement a worse index and hide the point of the example.
- **Ports 16400 and 6399 belong to this example.** Nothing here touches a Redis
  or `emb` you already have running; if either port is occupied by something the
  example did not start, it refuses rather than talking to it.
