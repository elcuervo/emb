-- between.lua: blend two passages and find what lies between them.
--
-- The atlas's one extra instrument: pick two passages and see the passages the
-- midpoint lands nearest, which is the plainest possible demonstration that the
-- space is continuous and that meaning has a geometry rather than a lookup.
--
--   EMB.EVSHA minilm <sha> 9 "the sea closed over the ship" "a guilty conscience" "c1" ... "c7"
--   -> { {rank=1, doc=5, score=0.71...}, ... }
--
-- KEYS[1] and KEYS[2] are the two passages to blend; KEYS[3..] are the corpus
-- items to compare against. The blend is the arithmetic midpoint of the two
-- embeddings — emb.math.add then emb.math.scale, both host-side over packed
-- float32 — and every text is scored by cosine to it.
--
-- As in rank.lua, the server requires one returned value per text and the
-- bridge charges one work unit per declared text, so the reply ranks every KEY
-- and `doc` is the text's 1-based position in the request. The two operands are
-- usually near the top (each is half of the midpoint); `rank` is what the page
-- reads, and it labels the operands as such. At most eight texts, so at most
-- six corpus candidates (design.md, D6).

local vecs = emb.embed(KEYS, { bytes = true })

local mid = emb.math.scale(emb.math.add(vecs[1], vecs[2]), 0.5)

local scores = {}
for i = 1, #KEYS do
  scores[i] = emb.similarity(mid, vecs[i], "cosine")
end

local ranked = emb.math.topk(scores, #KEYS)

local out = {}
for rank = 1, #ranked do
  out[rank] = { rank = rank, doc = ranked[rank].index, score = ranked[rank].value }
end
return out
