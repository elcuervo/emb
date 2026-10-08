-- clap_text.lua: text -> the CLAP text tower's L2-normalized 512-d embedding.
--
-- Probed contract (models/clap/text_model_quantized.onnx): the graph takes
-- ONLY `input_ids` — there is no attention_mask input, contrary to the export's
-- card — and its dimensions are dynamic. Output `text_embeds` is the projected
-- embedding and is NOT L2-normalized by the graph, so it is normalized here so
-- a cosine is a plain dot product wherever it is scored.

local MAXLEN = 77

local function normalize(bytes)
  local n = emb.math.norm(bytes)
  if n > 0 then
    return emb.math.float32_bytes(emb.math.scale(bytes, 1.0 / n))
  end
  return bytes
end

local function embed(text)
  local enc = emb.tokenize.encode(text, MAXLEN)
  local r = emb.run({
    input_ids = { shape = { 1, #enc.ids }, data = enc.ids },
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
