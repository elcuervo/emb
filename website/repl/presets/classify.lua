-- classify.lua: the classification demonstration (Xenova/distilbert-base-
-- uncased-finetuned-sst-2-english: input_ids + attention_mask -> logits [-1,2]).
--
-- It is the sandbox's own copy of the sequence-classification recipe, kept
-- beside the configuration that preloads it so the digest the site shows is the
-- digest of the bytes the server loaded. The labels arrive in the model's
-- training order as ARGV, so no label list is baked in.
--
--   EMB.EVSHA sst2 <sha> 1 "this film is great" NEGATIVE POSITIVE
--   -> { label = "POSITIVE", confidence = 0.99, scores = {...} }

local labels = {}
for i = 1, #ARGV do labels[i] = ARGV[i] end

-- Tokenizer's own pipeline (special tokens included).
local enc = emb.tokenize.encode(KEYS[1], 512)

-- Named-tensor inference against the model's graph.
local out = emb.run({
  input_ids      = { shape = {1, #enc.ids}, data = enc.ids },
  attention_mask = { shape = {1, #enc.ids}, data = enc.mask },
})

-- Stable softmax + first-maximum argmax.
local probs = emb.math.softmax(out.logits.data)
local idx, score = emb.math.argmax(probs)

-- Labels are positional (ARGV[i] <-> class i); reject a short list rather than
-- silently dropping the label field.
if #labels ~= #probs then
  return { err = string.format("expected %d labels, got %d", #probs, #labels) }
end

return { label = labels[idx], confidence = score, scores = probs }
