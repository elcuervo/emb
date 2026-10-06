-- emoji.lua — compose a query's terms into one vector.
--
-- The zone parses a name and spells its vocabulary words out, so the terms
-- arrive already resolved: KEYS[1] is the base term, and ARGV carries the rest
-- as (operator, text) pairs, in the order the query wrote them. Each term is
-- embedded through the server's own embedding path — the same batcher, the same
-- cache, the same ORT sessions as EMB — and folded left to right:
--
--     shark-fish+bird.dns.emb.is
--       KEYS[1] = "shark"
--       ARGV    = {"-", "fish", "+", "bird"}
--
-- The reply is one packed little-endian float32 vector: the query's direction,
-- which the zone then ranks the vocabulary against. A single term is that
-- term's own vector, unchanged.

local terms = { KEYS[1] }
local operators = { "+" }
for i = 1, #ARGV, 2 do
  operators[#operators + 1] = ARGV[i]
  terms[#terms + 1] = ARGV[i + 1]
end

local vectors = emb.embed(terms, { bytes = true })
local composed = vectors[1]
for i = 2, #terms do
  local vector = vectors[i]
  if operators[i] == "-" then
    -- Subtraction is an addition of the negated operand: the host surface has
    -- no `sub`, and packing the negated vector keeps the accumulator in the
    -- bulk form `emb.math.add` accepts without a per-element Lua loop.
    vector = emb.math.float32_bytes(emb.math.scale(vector, -1))
  end
  composed = emb.math.float32_bytes(emb.math.add(composed, vector))
end

return composed
