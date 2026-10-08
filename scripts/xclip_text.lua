-- xclip_text.lua: text -> the X-CLIP text tower's L2-normalized 512-d embedding.
--
-- Probed contract (models/xclip/text_tower.onnx): `input_ids` and
-- `attention_mask`, both FIXED [1, 77] int64 — the export declares no dynamic
-- axes — out `text_embeds` [1, 512], not normalized by the graph.

local MAXLEN = 77

local function normalize(bytes)
  local n = emb.math.norm(bytes)
  if n > 0 then
    return emb.math.float32_bytes(emb.math.scale(bytes, 1.0 / n))
  end
  return bytes
end

local function pad77(ids, mask)
  local outIds, outMask = {}, {}
  for i = 1, MAXLEN do
    outIds[i] = ids[i] or 0
    outMask[i] = mask[i] or 0
  end
  return outIds, outMask
end

local function embed(text)
  local enc = emb.tokenize.encode(text, MAXLEN)
  local ids, mask = pad77(enc.ids, enc.mask)
  local r = emb.run({
    input_ids = { shape = { 1, MAXLEN }, data = ids },
    attention_mask = { shape = { 1, MAXLEN }, data = mask },
  }, { outputs = { "text_embeds" }, bytes = true })
  return { shape = r.text_embeds.shape, bytes = normalize(r.text_embeds.bytes), dtype = "f32" }
end

if #KEYS == 1 then
  return embed(KEYS[1])
end
local out = {}
for i = 1, #KEYS do
  out[i] = embed(KEYS[i])
end
return out
