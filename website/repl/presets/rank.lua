-- rank.lua: rerank candidate passages against a query by embedding cosine.
--
-- The search plate's second opinion: `vec0` returns approximate neighbours, and
-- this re-orders them with the model itself. No second model, no cross-encoder.
--
--   EMB.EVSHA minilm <sha> 8 "the sea closed over the ship" "passage one" ... "passage seven"
--   -> { {rank=1, doc=1, score=0.99999994},   -- the query, against itself
--        {rank=2, doc=4, score=0.6231...},    -- the nearest candidate, ...
--        ... }
--
-- KEYS[1] is the query; KEYS[2..] are the candidates. The server runs one
-- evaluation with every text as a KEY and requires one returned value per text,
-- so the reply is a ranked entry per text: the query is rank 1 at cosine ~1 and
-- the candidates follow in descending score order. `doc` is the text's 1-based
-- position in the request, which is how the page maps an entry back to the
-- passage it sent.
--
-- Candidates live in KEYS rather than ARGV on purpose: the bridge charges one
-- work unit per *declared text* (`textsIn`, website/repl/limits.go), so a rerank
-- that hid its candidates in ARGV would buy ~60 embeddings for one unit. Both
-- the server and the bridge cap a request at 8 texts, which is why a rerank
-- carries at most seven candidates (design.md, D6).

local vecs = emb.embed(KEYS, { bytes = true })

-- One cosine per text against the query. At most eight texts, so the
-- interpreted loop is bounded; the vectors themselves never enter Lua (packed
-- float32), and the ordering is done host-side by emb.math.topk.
local scores = {}
for i = 1, #KEYS do
  scores[i] = emb.similarity(vecs[1], vecs[i], "cosine")
end

local ranked = emb.math.topk(scores, #KEYS)

local out = {}
for rank = 1, #ranked do
  out[rank] = { rank = rank, doc = ranked[rank].index, score = ranked[rank].value }
end
return out
