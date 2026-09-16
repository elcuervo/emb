-- embed.lua: the embedding demonstration, as a structured non-embedding reply.
--
-- It calls the server's own embedding path (`emb.embed` shares the batcher and
-- the `model:text` cache with the EMB command), then reports the vector's
-- dimension and L2 norm, plus the cosine similarity to an optional reference
-- text passed as the first ARGV argument. Nothing here returns 384 floats, so
-- the reply stays renderable as a hash.
--
--   EMB.EVSHA minilm <sha> 1 "hello world"
--   EMB.EVSHA minilm <sha> 1 "hello world" "hello there"
--   -> { dim = 384, norm = 1, similarity = 0.83 }

local packed = emb.embed(KEYS[1], { bytes = true })

local out = {
  dim = math.floor(#packed / 4),
  norm = emb.math.norm(packed),
}

if ARGV[1] then
  local other = emb.embed(ARGV[1], { bytes = true })
  out.similarity = emb.similarity(packed, other, "cosine")
end

return out
