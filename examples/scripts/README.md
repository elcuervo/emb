# Example scripts

Scripts are classified as either **reference** or **snippet**.

## `reference/` — maintained

Exercised by CI against a downloaded testbed model, and kept working across
releases. Treat these as the canonical implementation of their task.

| Script | Model | What it does |
|---|---|---|
| `gliner2.lua` | `cuerbot/gliner2-multi-v1` (int8) | GLiNER2 span extraction. Reads the logits tensor in its **packed** form and reduces it with host math, so the decode never runs element-by-element in Lua. |

Run the reference GLiNER2 script:

```bash
SHA=$(redis-cli EMB.SCRIPT LOAD gliner2 "$(cat examples/scripts/reference/gliner2.lua)")
redis-cli EMB.EVSHA gliner2 "$SHA" 1 "Apple CEO Tim Cook announced iPhone 15." PERSON ORG PRODUCT
```

## `snippets/` — illustrative

Demonstrate building blocks against specific model exports. They are useful
starting points, but they are not maintained as server components: a snippet
may assume a particular graph layout and can go stale.

| Script | What it demonstrates |
|---|---|
| `siglip2.lua` | Text embedding from a fused CLIP export (zeroed image branch) |
| `image_zeroshot.lua` | Cross-modal zero-shot classification with `emb.image.preprocess` |
| `sst2.lua` | Sequence classification from raw logits (`softmax` + `argmax`) |
| `qa.lua` | Extractive QA via `emb.tokenize.encode_pair` offsets |
| `rerank.lua` | Cross-encoder reranking with `sigmoid` scores |

## Writing production scripts

See the production scripting guide in the main `README.md` (Custom scripts →
Production notes). The short version:

- Use `emb.embed` / `emb.image.embed` for embeddings — they share the server's
  embedding cache, batcher and sessions, and never open a second model.
  `emb.image.embed` also shares the `EMB.IMG` content-addressed image cache and
  image session pool, so an image embedded by either path is a cache hit for the
  other and the same bytes never infer twice while caching is enabled.
- Image resources are opened **lazily**: `emb.image.info` and
  `emb.image.preprocess` resolve only the preprocessing plan, and the image
  session pool opens on the first `emb.image.embed`. A script that never touches
  `emb.image.*` allocates no image resources at all.
- Use `emb.run` / `emb.run_batch` with `{bytes = true, outputs = {...}}` for raw
  graph tensors, and reduce them with `emb.math.*` rather than interpreted Lua
  loops.
- Use `emb.similarity` / `emb.distance` (not hand-rolled cosine) so the semantics
  stay consistent across scripts.
- Scripts must be pure compute (identical inputs → identical replies); that is
  what makes the reply cache sound.
