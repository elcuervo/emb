-- sst2.lua: text classification example (Xenova/distilbert-base-uncased-
-- finetuned-sst-2-english: input_ids + attention_mask -> logits [-1, 2]).
-- Demonstrates the building blocks for classification models: plain encode,
-- emb.run, and the emb.math baseline (softmax + argmax).
--
--   EMB.EVSHA sst2 <sha> 1 "this film is great" NEGATIVE POSITIVE
--   -> hash {label = "POSITIVE", confidence = 0.97, scores = {...}}

local labels = {}
for i = 1, #ARGV do labels[i] = ARGV[i] end

-- Block: tokenizer's own pipeline (special tokens included) + offsets.
local enc = emb.tokenize.encode(KEYS[1], 512)

-- Block: named-tensor inference.
local out = emb.run({
  input_ids      = { shape = {1, #enc.ids}, data = enc.ids },
  attention_mask = { shape = {1, #enc.ids}, data = enc.mask },
})

-- Baseline: stable softmax + first-maximum argmax.
local probs = emb.math.softmax(out.logits.data)
local idx, score = emb.math.argmax(probs)

-- Model logic: labels arrive in the model's training order (ARGV[i] <-> class i).
return { label = labels[idx], confidence = score, scores = probs }