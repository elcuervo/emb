-- image_zeroshot.lua: zero-shot image classification on a fused CLIP/SigLIP export.
--
-- The server preprocesses the raw image bytes into the model's pixel_values
-- tensor (emb.image.preprocess), embeds the image and each candidate text
-- prompt in the same space, and returns the label with the highest cosine
-- similarity. Everything is pure compute: no network, deterministic, cacheable.
--
--   EMB.EVSHA siglip2 <sha> 1 <cat.jpg bytes> "a photo of a cat" "a photo of a dog"
--   -> { label = "a photo of a cat", score = 0.87, scores = {0.87, 0.12} }
--
-- KEYS[1] = raw image bytes; ARGV[1..n] = candidate labels.

local spec = emb.image.preprocess(KEYS[1])

local imageOut = emb.run({
  [spec.input] = { shape = spec.shape, bytes = spec.bytes, dtype = spec.dtype },
})
local image = imageOut.image_embeds.data

local function cosine(a, b)
  local dot, na, nb = 0, 0, 0
  for i = 1, #a do
    dot = dot + a[i] * b[i]
    na = na + a[i] * a[i]
    nb = nb + b[i] * b[i]
  end
  local denom = math.sqrt(na) * math.sqrt(nb)
  if denom == 0 then
    return 0
  end
  return dot / denom
end

local scores = {}
local best, bestScore = nil, -math.huge
for i = 1, #ARGV do
  local enc = emb.tokenize.encode("a photo of a " .. ARGV[i], 64)
  local textOut = emb.run({
    input_ids = { shape = { 1, #enc.ids }, data = enc.ids },
    -- The image branch is unused for text; fill builds the constant tensor
    -- host-side so no per-element table is constructed.
    [spec.input] = { shape = spec.shape, fill = 0, dtype = "f32" },
  })
  local similarity = cosine(image, textOut.text_embeds.data)
  scores[i] = similarity
  if similarity > bestScore then
    best, bestScore = ARGV[i], similarity
  end
end

return { label = best, score = bestScore, scores = scores }
