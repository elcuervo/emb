-- rerank.lua: cross-encoder reranking example (Xenova/bge-reranker-base:
-- input_ids + attention_mask -> logits [-1, 1]).
-- Demonstrates: repeated emb.run within one evaluation (the session persists),
-- the pair template, and the math baseline's scalar sigmoid.
--
--   EMB.EVSHA rerank <sha> 1 "what is the capital of france" "Paris" "Lyon" "Nice"
--   -> array of hashes sorted by score: {rank, doc, score}

local query = KEYS[1]
local results = {}

for i, doc in ipairs(ARGV) do
  local enc = emb.tokenize.encode_pair(query, doc, 512)
  local out = emb.run({
    input_ids      = { shape = {1, #enc.ids}, data = enc.ids },
    attention_mask = { shape = {1, #enc.ids}, data = enc.mask },
  })
  -- Baseline: scalar sigmoid over the single cross-encoder logit.
  local score = emb.math.sigmoid(out.logits.data[1])
  results[#results + 1] = { doc = doc, score = score }
end

-- Model logic: descending score order.
table.sort(results, function(a, b) return a.score > b.score end)
local ranked = {}
for i, r in ipairs(results) do
  ranked[i] = { rank = i, doc = r.doc, score = r.score }
end
return ranked