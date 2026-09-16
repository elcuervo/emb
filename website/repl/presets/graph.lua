-- graph.lua: a directed similarity graph over the texts it is handed.
--
-- This is the scripting layer doing the work the page cannot: one batched
-- embedding call, an N×N cosine matrix, and a host-side top-k per node — all
-- beside the model, returning edges rather than the N×N numbers.
--
--   EMB.EVSHA minilm <sha> 8 "p1" … "p8" "a query"
--   -> { {from=1, edges={{to=4,score=0.71},{to=2,score=0.66}}, query=0.42}, … }
--
-- One value per key, because a multi-text evaluation returns one value per text.
-- Each node carries its outgoing edges (its nearest neighbours, self excluded)
-- and, when ARGV[1] is present, its similarity to the query.

-- Two outgoing edges per node: sixteen edges over eight nodes is a graph a
-- reader can follow, where eight is a hairball.
local K = 2

local vecs = emb.embed(KEYS, { bytes = true })

local query = nil
if ARGV[1] then
  query = emb.embed({ ARGV[1] }, { bytes = true })[1]
end

local out = {}
for i = 1, #KEYS do
  local scores = {}
  for j = 1, #KEYS do
    -- A node is not its own neighbour; a sentinel below every cosine keeps the
    -- self edge out of the top-k rather than special-casing the loop.
    scores[j] = (i == j) and -1 or emb.similarity(vecs[i], vecs[j], "cosine")
  end
  local top = emb.math.topk(scores, K)
  local edges = {}
  for rank = 1, #top do
    edges[rank] = { to = top[rank].index, score = top[rank].value }
  end
  out[i] = {
    from = i,
    edges = edges,
    query = query and emb.similarity(vecs[i], query, "cosine") or 0,
  }
end
return out
