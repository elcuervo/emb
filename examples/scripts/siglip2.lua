-- siglip2.lua: text embedding example on a FUSED CLIP export
-- (onnx-community/siglip2-base-patch16-224-ONNX text_model: inputs input_ids
-- + pixel_values, outputs text_embeds [-1, 768]). The image branch is not
-- present in text requests, so pixel_values is fed as zeros; text_embeds
-- depends only on the text tokens.
--
--   EMB.EVSHA siglip2 <sha> 1 "a photo of a cat" NORMALIZE
--   -> hash {dim = 768, embedding = {...}}
--
-- KEYS[1] = text; ARGV[1] optional "normalize" to L2-normalize the vector.

local enc = emb.tokenize.encode(KEYS[1], 256)
local normalize = ARGV[1] == "normalize"

-- Feed the image branch a zeroed 1x3x224x224 tensor (not used for text).
local zeros = {}
for i = 1, 3 * 224 * 224 do zeros[i] = 0 end

local out = emb.run({
  input_ids    = { shape = {1, #enc.ids}, data = enc.ids },
  pixel_values = { shape = {1, 3, 224, 224}, data = zeros, dtype = "f32" },
})
local vec = out.text_embeds.data -- 768 floats

if normalize then
  local norm = 0
  for i = 1, #vec do norm = norm + vec[i] * vec[i] end
  norm = math.sqrt(norm)
  if norm > 0 then
    for i = 1, #vec do vec[i] = vec[i] / norm end
  end
end

return { dim = #vec, embedding = vec }