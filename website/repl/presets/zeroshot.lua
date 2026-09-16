-- zeroshot.lua: raw image bytes in, a label distribution out, on the sandbox's
-- fused CLIP export. This is the one preset allowed to receive a binary
-- argument: `EMB.EVSHA clip <sha> 1 <image bytes> <label...>`.
--
-- The server preprocesses the image with the model's own plan, embeds it and
-- each candidate label in the shared CLIP space, and softmaxes the cosines.
--
--   EMB.EVSHA clip <sha> 1 <image bytes> raven storm portrait
--   -> { label = "raven", score = 0.31, probs = {0.31, 0.2, 0.1}, scores = {...} }
--
-- The fused export carries both branches in one graph and requires every input
-- for either run, so the branch not in use gets a constant tensor built
-- host-side (`fill = 0` builds ~150k zeros without a Lua table). The graph
-- itself is asked for one output per run (`outputs = { ... }`), so no
-- unrequested branch is materialized.

-- A label is one more graph run after the image, so the call is bounded here as
-- well as by the bridge's argv cap: eight labels is the most the plate offers.
if #ARGV > 8 then
  return { err = "zeroshot takes at most eight labels" }
end

local spec = emb.image.preprocess(KEYS[1])

-- The text branch's own inputs for a run that only wants the image embedding.
local dummy = emb.tokenize.encode("a", 77)

local function inputs(ids, mask, pixels)
  return {
    input_ids = { shape = { 1, #ids }, data = ids },
    attention_mask = { shape = { 1, #mask }, data = mask },
    [spec.input] = pixels,
  }
end

local zero = { shape = spec.shape, fill = 0, dtype = "f32" }
local pixels = { shape = spec.shape, bytes = spec.bytes, dtype = spec.dtype }

local image = emb.run(
  inputs(dummy.ids, dummy.mask, pixels),
  { outputs = { "image_embeds" }, bytes = true }
).image_embeds.bytes

local scores = {}
for i = 1, #ARGV do
  local enc = emb.tokenize.encode("a photo of a " .. ARGV[i], 64)
  local text = emb.run(
    inputs(enc.ids, enc.mask, zero),
    { outputs = { "text_embeds" }, bytes = true }
  ).text_embeds.bytes
  scores[i] = emb.similarity(image, text, "cosine")
end

-- CLIP is trained with a learned logit scale (≈100): the cosine is a
-- similarity, and the scale is what turns it into a distribution with a winner.
-- Softmaxing the raw cosines flattens every label to about 1/n and hides the
-- answer, so the scale is applied here, the way the model's own logits do.
local LOGIT_SCALE = 100
local probs = emb.math.softmax(emb.math.scale(scores, LOGIT_SCALE))
local best, bestScore = nil, -math.huge
for i = 1, #ARGV do
  if scores[i] > bestScore then
    best, bestScore = ARGV[i], scores[i]
  end
end

return { label = best, score = bestScore, probs = probs, scores = scores }
